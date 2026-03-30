package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"douyin-live-record/internal/model"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`CREATE TABLE IF NOT EXISTS app_user (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS app_session (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS app_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			streamer_name TEXT NOT NULL,
			room_url TEXT NOT NULL,
			auto_record_enabled INTEGER NOT NULL,
			poll_interval_seconds INTEGER NOT NULL,
			stream_quality TEXT NOT NULL,
			segment_minutes INTEGER NOT NULL,
			save_subdir TEXT NOT NULL,
			keep_days INTEGER NOT NULL,
			min_free_gb INTEGER NOT NULL,
			cleanup_to_gb INTEGER NOT NULL,
			cookies_file TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS record_session (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			streamer_name TEXT NOT NULL,
			room_url TEXT NOT NULL,
			save_subdir TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			final_file_path TEXT NOT NULL,
			file_size_bytes INTEGER NOT NULL,
			error_message TEXT NOT NULL,
			quality TEXT NOT NULL,
			segment_minutes INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS record_segment (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			record_session_id INTEGER NOT NULL,
			segment_index INTEGER NOT NULL,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			file_path TEXT NOT NULL,
			merged INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS service_event (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			level TEXT NOT NULL,
			event_type TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}

	if _, err := s.db.Exec(`INSERT OR IGNORE INTO app_config (
			id, streamer_name, room_url, auto_record_enabled, poll_interval_seconds, stream_quality,
			segment_minutes, save_subdir, keep_days, min_free_gb, cleanup_to_gb, cookies_file, updated_at
		) VALUES (1, '', '', 0, 30, 'best', 15, '/default', 7, 8, 12, '', ?);`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}

	return nil
}

func (s *Store) EnsureAdmin(ctx context.Context, username, passwordHash string) error {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM app_user WHERE username = ?`, username).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO app_user(username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, passwordHash, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (model.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, created_at FROM app_user WHERE username = ?`, username)
	return scanUser(row)
}

func (s *Store) CreateSession(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_session(user_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)`, userID, hashToken(token), expiresAt.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339))
	return err
}

func (s *Store) GetSessionByToken(ctx context.Context, token string) (model.Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, user_id, token_hash, expires_at, created_at, last_seen_at
		FROM app_session WHERE token_hash = ?`, hashToken(token))
	return scanSession(row)
}

func (s *Store) TouchSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE app_session SET last_seen_at = ? WHERE token_hash = ?`,
		time.Now().UTC().Format(time.RFC3339), hashToken(token))
	return err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_session WHERE token_hash = ?`, hashToken(token))
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_session WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) LoadConfig(ctx context.Context) (model.AppConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT streamer_name, room_url, auto_record_enabled, poll_interval_seconds, stream_quality,
			segment_minutes, save_subdir, keep_days, min_free_gb, cleanup_to_gb, cookies_file, updated_at
		FROM app_config WHERE id = 1`)
	return scanConfig(row)
}

func (s *Store) SaveConfig(ctx context.Context, cfg model.AppConfig) (model.AppConfig, error) {
	cfg.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE app_config SET streamer_name = ?, room_url = ?, auto_record_enabled = ?, poll_interval_seconds = ?,
		stream_quality = ?, segment_minutes = ?, save_subdir = ?, keep_days = ?, min_free_gb = ?, cleanup_to_gb = ?, cookies_file = ?, updated_at = ?
		WHERE id = 1`,
		cfg.StreamerName, cfg.RoomURL, boolToInt(cfg.AutoRecordEnabled), cfg.PollIntervalSeconds, cfg.StreamQuality, cfg.SegmentMinutes,
		cfg.SaveSubdir, cfg.KeepDays, cfg.MinFreeGB, cfg.CleanupToGB, cfg.CookiesFile, cfg.UpdatedAt.Format(time.RFC3339))
	return cfg, err
}

func (s *Store) CreateRecordSession(ctx context.Context, session model.RecordSession) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO record_session(streamer_name, room_url, save_subdir, status, started_at, ended_at, final_file_path,
		file_size_bytes, error_message, quality, segment_minutes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.StreamerName, session.RoomURL, session.SaveSubdir, session.Status, session.StartedAt.Format(time.RFC3339),
		nullableTime(session.EndedAt), session.FinalFilePath, session.FileSizeBytes, session.ErrorMessage, session.Quality, session.SegmentMinutes)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateRecordSession(ctx context.Context, session model.RecordSession) error {
	_, err := s.db.ExecContext(ctx, `UPDATE record_session SET streamer_name = ?, room_url = ?, save_subdir = ?, status = ?, started_at = ?,
		ended_at = ?, final_file_path = ?, file_size_bytes = ?, error_message = ?, quality = ?, segment_minutes = ? WHERE id = ?`,
		session.StreamerName, session.RoomURL, session.SaveSubdir, session.Status, session.StartedAt.Format(time.RFC3339),
		nullableTime(session.EndedAt), session.FinalFilePath, session.FileSizeBytes, session.ErrorMessage, session.Quality, session.SegmentMinutes, session.ID)
	return err
}

func (s *Store) GetRecordSession(ctx context.Context, id int64) (model.RecordSession, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, streamer_name, room_url, save_subdir, status, started_at, ended_at, final_file_path,
			file_size_bytes, error_message, quality, segment_minutes FROM record_session WHERE id = ?`, id)
	return scanRecordSession(row)
}

func (s *Store) ListRecordSessions(ctx context.Context, limit int) ([]model.RecordSession, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, streamer_name, room_url, save_subdir, status, started_at, ended_at, final_file_path,
			file_size_bytes, error_message, quality, segment_minutes
		FROM record_session ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.RecordSession
	for rows.Next() {
		session, err := scanRecordSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Store) GetLatestUnfinishedSession(ctx context.Context) (*model.RecordSession, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, streamer_name, room_url, save_subdir, status, started_at, ended_at, final_file_path,
			file_size_bytes, error_message, quality, segment_minutes
		FROM record_session WHERE status IN (?, ?, ?, ?) ORDER BY started_at DESC LIMIT 1`,
		model.ServiceStateRecording, model.ServiceStateStopping, model.ServiceStateMerging, model.ServiceStateError)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	session, err := scanRecordSession(rows)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Store) ReplaceSegments(ctx context.Context, recordSessionID int64, segments []model.RecordSegment) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM record_segment WHERE record_session_id = ?`, recordSessionID); err != nil {
		return err
	}
	for _, segment := range segments {
		if _, err = tx.ExecContext(ctx, `INSERT INTO record_segment(record_session_id, segment_index, started_at, ended_at, file_path, merged)
			VALUES (?, ?, ?, ?, ?, ?)`,
			recordSessionID, segment.SegmentIndex, segment.StartedAt.Format(time.RFC3339), nullableTime(segment.EndedAt), segment.FilePath, boolToInt(segment.Merged)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListSegments(ctx context.Context, recordSessionID int64) ([]model.RecordSegment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, record_session_id, segment_index, started_at, ended_at, file_path, merged
		FROM record_segment WHERE record_session_id = ? ORDER BY segment_index ASC`, recordSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var segments []model.RecordSegment
	for rows.Next() {
		var (
			segment model.RecordSegment
			started string
			ended   sql.NullString
			merged  int
		)
		if err := rows.Scan(&segment.ID, &segment.RecordSessionID, &segment.SegmentIndex, &started, &ended, &segment.FilePath, &merged); err != nil {
			return nil, err
		}
		segment.StartedAt, err = time.Parse(time.RFC3339, started)
		if err != nil {
			return nil, err
		}
		if ended.Valid {
			t, err := time.Parse(time.RFC3339, ended.String)
			if err != nil {
				return nil, err
			}
			segment.EndedAt = &t
		}
		segment.Merged = merged == 1
		segments = append(segments, segment)
	}
	return segments, rows.Err()
}

func (s *Store) AddEvent(ctx context.Context, level, eventType, message string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO service_event(level, event_type, message, created_at) VALUES (?, ?, ?, ?)`,
		level, eventType, message, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) ListEvents(ctx context.Context, limit int) ([]model.ServiceEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, level, event_type, message, created_at FROM service_event ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []model.ServiceEvent
	for rows.Next() {
		var event model.ServiceEvent
		var created string
		if err := rows.Scan(&event.ID, &event.Level, &event.EventType, &event.Message, &created); err != nil {
			return nil, err
		}
		event.CreatedAt, err = time.Parse(time.RFC3339, created)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (model.User, error) {
	var user model.User
	var created string
	if err := s.Scan(&user.ID, &user.Username, &user.PasswordHash, &created); err != nil {
		return model.User{}, err
	}
	parsed, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return model.User{}, err
	}
	user.CreatedAt = parsed
	return user, nil
}

func scanSession(s scanner) (model.Session, error) {
	var session model.Session
	var expires, created, lastSeen string
	if err := s.Scan(&session.ID, &session.UserID, &session.TokenHash, &expires, &created, &lastSeen); err != nil {
		return model.Session{}, err
	}
	var err error
	session.ExpiresAt, err = time.Parse(time.RFC3339, expires)
	if err != nil {
		return model.Session{}, err
	}
	session.CreatedAt, err = time.Parse(time.RFC3339, created)
	if err != nil {
		return model.Session{}, err
	}
	session.LastSeenAt, err = time.Parse(time.RFC3339, lastSeen)
	if err != nil {
		return model.Session{}, err
	}
	return session, nil
}

func scanConfig(s scanner) (model.AppConfig, error) {
	var (
		cfg     model.AppConfig
		enabled int
		updated string
	)
	if err := s.Scan(&cfg.StreamerName, &cfg.RoomURL, &enabled, &cfg.PollIntervalSeconds, &cfg.StreamQuality,
		&cfg.SegmentMinutes, &cfg.SaveSubdir, &cfg.KeepDays, &cfg.MinFreeGB, &cfg.CleanupToGB, &cfg.CookiesFile, &updated); err != nil {
		return model.AppConfig{}, err
	}
	cfg.AutoRecordEnabled = enabled == 1
	parsed, err := time.Parse(time.RFC3339, updated)
	if err != nil {
		return model.AppConfig{}, err
	}
	cfg.UpdatedAt = parsed
	return cfg, nil
}

func scanRecordSession(s scanner) (model.RecordSession, error) {
	var (
		session model.RecordSession
		started string
		ended   sql.NullString
	)
	if err := s.Scan(&session.ID, &session.StreamerName, &session.RoomURL, &session.SaveSubdir, &session.Status, &started,
		&ended, &session.FinalFilePath, &session.FileSizeBytes, &session.ErrorMessage, &session.Quality, &session.SegmentMinutes); err != nil {
		return model.RecordSession{}, err
	}
	parsed, err := time.Parse(time.RFC3339, started)
	if err != nil {
		return model.RecordSession{}, err
	}
	session.StartedAt = parsed
	if ended.Valid {
		t, err := time.Parse(time.RFC3339, ended.String)
		if err != nil {
			return model.RecordSession{}, err
		}
		session.EndedAt = &t
	}
	return session, nil
}
