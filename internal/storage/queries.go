package storage

import (
	"context"

	"douyin-live-record/internal/model"
)

func (s *Store) ListPurgeCandidates(ctx context.Context, limit int) ([]model.RecordSession, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, streamer_name, room_url, save_subdir, status, started_at, ended_at, final_file_path,
			file_size_bytes, error_message, quality, segment_minutes
		FROM record_session WHERE status = 'completed' AND final_file_path <> ''
		ORDER BY started_at ASC LIMIT ?`, limit)
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

func (s *Store) GetLatestMergeCandidate(ctx context.Context) (*model.RecordSession, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, streamer_name, room_url, save_subdir, status, started_at, ended_at, final_file_path,
			file_size_bytes, error_message, quality, segment_minutes
		FROM record_session WHERE status IN ('error', 'merging', 'stopping', 'recording')
		ORDER BY started_at DESC LIMIT 1`)
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
