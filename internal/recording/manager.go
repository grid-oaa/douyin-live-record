package recording

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"douyin-live-record/internal/model"
	"douyin-live-record/internal/storage"
)

type Manager struct {
	store          *storage.Store
	logger         *slog.Logger
	probe          LiveProbe
	recorder       Recorder
	recordingsRoot string
	cookiesRoot    string
	probeTimeout   time.Duration

	mu      sync.RWMutex
	cfg     model.AppConfig
	state   string
	message string
	lastChk *time.Time
	active  *activeRecording

	wakeCh   chan struct{}
	stopCh   chan struct{}
	stopped  chan struct{}
	resultCh chan recordingResult
}

type activeRecording struct {
	session       model.RecordSession
	sessionConfig model.AppConfig
	sessionDir    string
	partsDir      string
	finalPath     string
	cookieHeader  string
	handle        Handle
	offlineCount  int
	stopRequested bool
}

type recordingResult struct {
	sessionID int64
	err       error
}

func NewManager(store *storage.Store, logger *slog.Logger, probe LiveProbe, recorder Recorder, recordingsRoot, cookiesRoot string, probeTimeout time.Duration) (*Manager, error) {
	cfg, err := store.LoadConfig(context.Background())
	if err != nil {
		return nil, err
	}
	return &Manager{
		store:          store,
		logger:         logger,
		probe:          probe,
		recorder:       recorder,
		recordingsRoot: recordingsRoot,
		cookiesRoot:    cookiesRoot,
		probeTimeout:   probeTimeout,
		cfg:            cfg,
		state:          initialState(cfg),
		message:        initialMessage(cfg),
		wakeCh:         make(chan struct{}, 1),
		stopCh:         make(chan struct{}),
		stopped:        make(chan struct{}),
		resultCh:       make(chan recordingResult, 4),
	}, nil
}

func (m *Manager) Start() {
	go m.loop()
}

func (m *Manager) Stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
	<-m.stopped
}

func (m *Manager) CurrentConfig() model.AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) UpdateConfig(ctx context.Context, cfg model.AppConfig) (model.AppConfig, error) {
	cfg.SaveSubdir = strings.TrimSpace(cfg.SaveSubdir)
	normalizedSubdir, err := NormalizeSaveSubdir(cfg.SaveSubdir)
	if err != nil {
		return model.AppConfig{}, err
	}
	cfg.SaveSubdir = normalizedSubdir
	if _, err := ResolveCookieFile(m.cookiesRoot, cfg.CookiesFile); err != nil {
		return model.AppConfig{}, err
	}

	saved, err := m.store.SaveConfig(ctx, cfg)
	if err != nil {
		return model.AppConfig{}, err
	}
	m.mu.Lock()
	m.cfg = saved
	if m.active == nil {
		m.state = initialState(saved)
		m.message = initialMessage(saved)
	}
	m.mu.Unlock()
	m.logEvent(ctx, "info", "config.updated", "配置已保存并生效")
	m.wake()
	return saved, nil
}

func (m *Manager) SetAutoRecord(ctx context.Context, enabled bool) error {
	cfg := m.CurrentConfig()
	cfg.AutoRecordEnabled = enabled
	if _, err := m.UpdateConfig(ctx, cfg); err != nil {
		return err
	}
	if !enabled {
		m.requestStop(context.Background(), "手动停止录制")
	}
	return nil
}

func (m *Manager) RebuildLatest(ctx context.Context) error {
	session, err := m.store.GetLatestMergeCandidate(ctx)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("没有可重建的录制会话")
	}
	sessionDir := filepath.Dir(session.FinalFilePath)
	partsDir := filepath.Join(sessionDir, "parts")
	if _, err := os.Stat(partsDir); err != nil {
		return err
	}
	session.Status = model.ServiceStateMerging
	if err := m.store.UpdateRecordSession(ctx, *session); err != nil {
		return err
	}
	if err := m.recorder.Merge(ctx, partsDir, session.FinalFilePath); err != nil {
		session.Status = model.ServiceStateError
		session.ErrorMessage = err.Error()
		_ = m.store.UpdateRecordSession(ctx, *session)
		return err
	}
	info, err := os.Stat(session.FinalFilePath)
	if err == nil {
		session.FileSizeBytes = info.Size()
	}
	session.Status = "completed"
	session.ErrorMessage = ""
	now := time.Now().UTC()
	session.EndedAt = &now
	if err := m.store.UpdateRecordSession(ctx, *session); err != nil {
		return err
	}
	_ = os.RemoveAll(partsDir)
	m.logEvent(ctx, "info", "record.rebuild", fmt.Sprintf("会话 %d 已重新合并", session.ID))
	return nil
}

func (m *Manager) Status() model.RuntimeStatus {
	m.mu.RLock()
	cfg := m.cfg
	state := m.state
	message := m.message
	lastChk := m.lastChk
	var current *model.RecordSession
	var currentID *int64
	if m.active != nil {
		sessionCopy := m.active.session
		current = &sessionCopy
		id := sessionCopy.ID
		currentID = &id
	}
	m.mu.RUnlock()

	free, total := diskSpace(m.recordingsRoot)
	return model.RuntimeStatus{
		State:             state,
		Message:           message,
		LastCheckAt:       lastChk,
		CurrentSessionID:  currentID,
		CurrentSession:    current,
		CurrentConfig:     cfg,
		DiskFreeBytes:     free,
		DiskTotalBytes:    total,
		RecordingRoot:     m.recordingsRoot,
		AppliedConfigHint: model.ConfigApplyMatrix(),
	}
}

func (m *Manager) loop() {
	defer close(m.stopped)
	_ = os.MkdirAll(m.recordingsRoot, 0o755)
	_ = os.MkdirAll(m.cookiesRoot, 0o755)
	m.recoverSession()

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-m.stopCh:
			m.requestStop(context.Background(), "服务停止")
			if m.hasActive() {
				result := <-m.resultCh
				m.handleRecordingResult(result)
			}
			return
		case <-timer.C:
			m.tick()
			timer.Reset(m.nextInterval())
		case result := <-m.resultCh:
			m.handleRecordingResult(result)
			timer.Reset(0)
		case <-m.wakeCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(0)
		}
	}
}

func (m *Manager) nextInterval() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cfg.PollIntervalSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(m.cfg.PollIntervalSeconds) * time.Second
}

func (m *Manager) tick() {
	if err := m.cleanupExpiredData(context.Background()); err != nil {
		m.logEvent(context.Background(), "warn", "storage.cleanup", err.Error())
	}

	m.mu.RLock()
	active := m.active
	cfg := m.cfg
	m.mu.RUnlock()

	if active != nil {
		m.tickRecording(active)
		return
	}
	if !cfg.AutoRecordEnabled {
		m.setRuntime(model.ServiceStateDisabled, "已关闭自动检查录制")
		return
	}
	if strings.TrimSpace(cfg.RoomURL) == "" {
		m.setRuntime(model.ServiceStateIdle, "请先配置直播间地址")
		return
	}

	cookieHeader, err := loadCookieHeaderForConfig(m.cookiesRoot, cfg.CookiesFile)
	if err != nil {
		m.setRuntime(model.ServiceStateError, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.probeTimeout)
	defer cancel()
	result, err := m.probe.Probe(ctx, cfg.RoomURL, cfg.StreamQuality, cookieHeader)
	now := time.Now().UTC()
	m.mu.Lock()
	m.lastChk = &now
	m.mu.Unlock()
	if err != nil {
		m.setRuntime(model.ServiceStateError, err.Error())
		m.logEvent(context.Background(), "error", "probe.failed", err.Error())
		return
	}
	if !result.Live {
		m.setRuntime(model.ServiceStateIdle, "主播未开播，等待下一次检查")
		return
	}
	if err := m.startRecording(context.Background(), cfg, cookieHeader); err != nil {
		m.setRuntime(model.ServiceStateError, err.Error())
		m.logEvent(context.Background(), "error", "record.start_failed", err.Error())
		return
	}
}

func (m *Manager) tickRecording(active *activeRecording) {
	if active.stopRequested {
		m.setRuntime(model.ServiceStateStopping, "正在停止并收尾当前录制")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.probeTimeout)
	defer cancel()
	result, err := m.probe.Probe(ctx, active.session.RoomURL, active.session.Quality, active.cookieHeader)
	now := time.Now().UTC()
	m.mu.Lock()
	m.lastChk = &now
	m.mu.Unlock()
	if err != nil {
		m.logEvent(context.Background(), "warn", "probe.recording_failed", err.Error())
		return
	}
	if result.Live {
		m.mu.Lock()
		if m.active != nil {
			m.active.offlineCount = 0
		}
		m.mu.Unlock()
		m.setRuntime(model.ServiceStateRecording, "正在录制直播")
		return
	}
	m.mu.Lock()
	if m.active != nil {
		m.active.offlineCount++
		active = m.active
	}
	m.mu.Unlock()
	if active != nil && active.offlineCount >= 2 {
		m.requestStop(context.Background(), "检测到主播已下播，准备合并 MP4")
	}
}

func (m *Manager) startRecording(ctx context.Context, cfg model.AppConfig, cookieHeader string) error {
	if err := m.cleanupExpiredData(ctx); err != nil {
		return err
	}

	recordSession := model.RecordSession{
		StreamerName:   cfg.StreamerName,
		RoomURL:        cfg.RoomURL,
		SaveSubdir:     cfg.SaveSubdir,
		Status:         model.ServiceStateRecording,
		StartedAt:      time.Now().UTC(),
		Quality:        cfg.StreamQuality,
		SegmentMinutes: cfg.SegmentMinutes,
	}
	sessionID, err := m.store.CreateRecordSession(ctx, recordSession)
	if err != nil {
		return err
	}
	recordSession.ID = sessionID

	baseDir, err := ResolveSaveDir(m.recordingsRoot, cfg.SaveSubdir)
	if err != nil {
		return err
	}
	dateDir := recordSession.StartedAt.Format("2006-01-02")
	sessionDir := filepath.Join(baseDir, dateDir, fmt.Sprintf("session-%d", sessionID))
	partsDir := filepath.Join(sessionDir, "parts")
	finalPath := filepath.Join(sessionDir, "final.mp4")
	if err := os.MkdirAll(partsDir, 0o755); err != nil {
		return err
	}
	recordSession.FinalFilePath = finalPath
	if err := m.store.UpdateRecordSession(ctx, recordSession); err != nil {
		return err
	}

	handle, err := m.recorder.Start(ctx, StartOptions{
		RoomURL:        cfg.RoomURL,
		Quality:        cfg.StreamQuality,
		PartsDir:       partsDir,
		SegmentMinutes: cfg.SegmentMinutes,
		CookieHeader:   cookieHeader,
		StartIndex:     0,
	})
	if err != nil {
		recordSession.Status = model.ServiceStateError
		recordSession.ErrorMessage = err.Error()
		_ = m.store.UpdateRecordSession(ctx, recordSession)
		return err
	}

	active := &activeRecording{
		session:       recordSession,
		sessionConfig: cfg,
		sessionDir:    sessionDir,
		partsDir:      partsDir,
		finalPath:     finalPath,
		cookieHeader:  cookieHeader,
		handle:        handle,
	}
	m.mu.Lock()
	m.active = active
	m.state = model.ServiceStateRecording
	m.message = "正在录制直播"
	m.mu.Unlock()
	m.logEvent(ctx, "info", "record.started", fmt.Sprintf("开始录制会话 %d", sessionID))
	go m.waitRecording(active)
	return nil
}

func (m *Manager) waitRecording(active *activeRecording) {
	m.resultCh <- recordingResult{sessionID: active.session.ID, err: active.handle.Wait()}
}

func (m *Manager) handleRecordingResult(result recordingResult) {
	m.mu.RLock()
	active := m.active
	m.mu.RUnlock()
	if active == nil || active.session.ID != result.sessionID {
		return
	}
	if active.stopRequested {
		m.finalizeSession(active, result.err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.probeTimeout)
	defer cancel()
	probeResult, err := m.probe.Probe(ctx, active.session.RoomURL, active.session.Quality, active.cookieHeader)
	if err == nil && probeResult.Live {
		m.logEvent(context.Background(), "warn", "record.restarting", fmt.Sprintf("会话 %d 录制进程中断，准备续录", active.session.ID))
		if restartErr := m.restartRecording(active); restartErr == nil {
			return
		} else {
			result.err = errors.Join(result.err, restartErr)
		}
	}
	m.finalizeSession(active, result.err)
}

func (m *Manager) restartRecording(active *activeRecording) error {
	segments, err := CollectSegments(active.session.ID, active.partsDir, active.session.StartedAt)
	if err != nil {
		return err
	}
	if err := m.store.ReplaceSegments(context.Background(), active.session.ID, segments); err != nil {
		return err
	}
	handle, err := m.recorder.Start(context.Background(), StartOptions{
		RoomURL:        active.session.RoomURL,
		Quality:        active.session.Quality,
		PartsDir:       active.partsDir,
		SegmentMinutes: active.session.SegmentMinutes,
		CookieHeader:   active.cookieHeader,
		StartIndex:     len(segments),
	})
	if err != nil {
		return err
	}
	m.mu.Lock()
	if m.active != nil && m.active.session.ID == active.session.ID {
		m.active.handle = handle
		m.active.offlineCount = 0
	}
	m.mu.Unlock()
	go m.waitRecording(active)
	return nil
}

func (m *Manager) finalizeSession(active *activeRecording, procErr error) {
	m.setRuntime(model.ServiceStateMerging, "正在合并 MP4")

	segments, err := CollectSegments(active.session.ID, active.partsDir, active.session.StartedAt)
	if err == nil {
		err = m.store.ReplaceSegments(context.Background(), active.session.ID, segments)
	}
	session := active.session
	now := time.Now().UTC()
	session.EndedAt = &now
	if err == nil {
		err = m.recorder.Merge(context.Background(), active.partsDir, active.finalPath)
	}
	if err == nil {
		info, statErr := os.Stat(active.finalPath)
		if statErr == nil {
			session.FileSizeBytes = info.Size()
		}
		session.Status = "completed"
		session.ErrorMessage = ""
		_ = os.RemoveAll(active.partsDir)
		m.logEvent(context.Background(), "info", "record.completed", fmt.Sprintf("会话 %d 已完成", session.ID))
	} else {
		session.Status = model.ServiceStateError
		session.ErrorMessage = errors.Join(procErr, err).Error()
		m.logEvent(context.Background(), "error", "record.failed", session.ErrorMessage)
	}
	_ = m.store.UpdateRecordSession(context.Background(), session)

	m.mu.Lock()
	m.active = nil
	cfg := m.cfg
	m.mu.Unlock()
	if cfg.AutoRecordEnabled {
		m.setRuntime(model.ServiceStateIdle, "等待下一次检查")
	} else {
		m.setRuntime(model.ServiceStateDisabled, "已关闭自动检查录制")
	}
}

func (m *Manager) recoverSession() {
	session, err := m.store.GetLatestUnfinishedSession(context.Background())
	if err != nil || session == nil || session.FinalFilePath == "" {
		return
	}
	partsDir := filepath.Join(filepath.Dir(session.FinalFilePath), "parts")
	if _, err := os.Stat(partsDir); err != nil {
		return
	}
	if mergeErr := m.recorder.Merge(context.Background(), partsDir, session.FinalFilePath); mergeErr != nil {
		session.Status = model.ServiceStateError
		session.ErrorMessage = mergeErr.Error()
		_ = m.store.UpdateRecordSession(context.Background(), *session)
		m.setRuntime(model.ServiceStateError, "检测到未完成录制，需要手动重试合并")
		return
	}
	info, err := os.Stat(session.FinalFilePath)
	if err == nil {
		session.FileSizeBytes = info.Size()
	}
	session.Status = "completed"
	now := time.Now().UTC()
	session.EndedAt = &now
	session.ErrorMessage = ""
	_ = m.store.UpdateRecordSession(context.Background(), *session)
	_ = os.RemoveAll(partsDir)
}

func (m *Manager) requestStop(ctx context.Context, reason string) {
	m.mu.Lock()
	active := m.active
	if active == nil || active.stopRequested {
		m.mu.Unlock()
		return
	}
	active.stopRequested = true
	m.state = model.ServiceStateStopping
	m.message = reason
	m.mu.Unlock()

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := active.handle.Stop(stopCtx); err != nil {
		m.logEvent(context.Background(), "warn", "record.stop", err.Error())
	}
}

func (m *Manager) cleanupExpiredData(ctx context.Context) error {
	cfg := m.CurrentConfig()
	free, _ := diskSpace(m.recordingsRoot)
	candidates, err := m.store.ListPurgeCandidates(ctx, 200)
	if err != nil {
		return err
	}

	cutoff := time.Now().UTC().Add(-time.Duration(cfg.KeepDays) * 24 * time.Hour)
	for _, candidate := range candidates {
		if candidate.EndedAt != nil && candidate.EndedAt.Before(cutoff) {
			if err := os.Remove(candidate.FinalFilePath); err == nil || os.IsNotExist(err) {
				m.logEvent(ctx, "info", "storage.retention", fmt.Sprintf("删除超过保留期的录制：%s", candidate.FinalFilePath))
			}
		}
	}

	free, _ = diskSpace(m.recordingsRoot)
	if free >= uint64(cfg.MinFreeGB)*gigabyte {
		return nil
	}
	target := uint64(cfg.CleanupToGB) * gigabyte
	for _, candidate := range candidates {
		if free >= target {
			return nil
		}
		if candidate.FinalFilePath == "" {
			continue
		}
		if err := os.Remove(candidate.FinalFilePath); err != nil && !os.IsNotExist(err) {
			continue
		}
		free, _ = diskSpace(m.recordingsRoot)
		m.logEvent(ctx, "info", "storage.purged", fmt.Sprintf("删除旧录制文件：%s", candidate.FinalFilePath))
	}
	return nil
}

func (m *Manager) hasActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active != nil
}

func (m *Manager) wake() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

func (m *Manager) setRuntime(state, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = state
	m.message = message
}

func (m *Manager) logEvent(ctx context.Context, level, eventType, message string) {
	if err := m.store.AddEvent(ctx, level, eventType, message); err != nil {
		m.logger.Warn("failed to persist event", "event", eventType, "error", err)
	}
	m.logger.Info(message, "event", eventType, "level", level)
}

func initialState(cfg model.AppConfig) string {
	if !cfg.AutoRecordEnabled {
		return model.ServiceStateDisabled
	}
	return model.ServiceStateIdle
}

func initialMessage(cfg model.AppConfig) string {
	if !cfg.AutoRecordEnabled {
		return "已关闭自动检查录制"
	}
	return "等待下一次检查"
}

func loadCookieHeaderForConfig(root, configured string) (string, error) {
	path, err := ResolveCookieFile(root, configured)
	if err != nil || path == "" {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(content), "\n")
	var cookies []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 7 {
			cookies = append(cookies, fields[5]+"="+fields[6])
		}
	}
	return strings.Join(cookies, "; "), nil
}

func diskSpace(path string) (free uint64, total uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	return stat.Bavail * uint64(stat.Bsize), stat.Blocks * uint64(stat.Bsize)
}

const gigabyte = 1024 * 1024 * 1024
