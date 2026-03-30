package recording

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"douyin-live-record/internal/model"
	"douyin-live-record/internal/storage"

	"log/slog"
)

type fakeProbe struct {
	mu      sync.Mutex
	results []bool
	idx     int
}

func (f *fakeProbe) Probe(ctx context.Context, roomURL, quality, cookieHeader string) (ProbeResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.results) == 0 {
		return ProbeResult{Live: false}, nil
	}
	if f.idx >= len(f.results) {
		return ProbeResult{Live: f.results[len(f.results)-1]}, nil
	}
	result := f.results[f.idx]
	f.idx++
	return ProbeResult{Live: result}, nil
}

type fakeRecorder struct {
	mu         sync.Mutex
	starts     []StartOptions
	stopCount  int
	mergeCount int
	handle     *fakeHandle
}

func (f *fakeRecorder) Start(ctx context.Context, opts StartOptions) (Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, opts)
	if err := os.MkdirAll(opts.PartsDir, 0o755); err != nil {
		return nil, err
	}
	part := filepath.Join(opts.PartsDir, "part-00000.ts")
	if err := os.WriteFile(part, []byte("ts"), 0o644); err != nil {
		return nil, err
	}
	handle := &fakeHandle{done: make(chan struct{})}
	f.handle = handle
	return handle, nil
}

func (f *fakeRecorder) Merge(ctx context.Context, partsDir, outputFile string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mergeCount++
	if err := os.WriteFile(outputFile, []byte("mp4"), 0o644); err != nil {
		return err
	}
	return nil
}

type fakeHandle struct {
	once sync.Once
	done chan struct{}
}

func (f *fakeHandle) Stop(ctx context.Context) error {
	f.once.Do(func() { close(f.done) })
	return nil
}

func (f *fakeHandle) Wait() error {
	<-f.done
	return nil
}

func TestManagerUpdateConfigAndStopRecording(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.New(filepath.Join(tempDir, "recorder.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	cfg := model.DefaultConfig()
	cfg.StreamerName = "主播A"
	cfg.RoomURL = "https://live.douyin.com/abc"
	cfg.AutoRecordEnabled = true
	cfg.PollIntervalSeconds = 1
	cfg.SaveSubdir = "/anchor-a"
	if _, err := store.SaveConfig(context.Background(), cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	probe := &fakeProbe{results: []bool{true, true, true}}
	recorder := &fakeRecorder{}
	manager, err := NewManager(store, slog.Default(), probe, recorder, filepath.Join(tempDir, "recordings"), filepath.Join(tempDir, "cookies"), time.Second)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	manager.Start()
	defer manager.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if status := manager.Status(); status.State == model.ServiceStateRecording {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if status := manager.Status(); status.State != model.ServiceStateRecording {
		t.Fatalf("expected recording state, got %s", status.State)
	}

	cfg.SaveSubdir = "/anchor-b"
	if _, err := manager.UpdateConfig(context.Background(), cfg); err != nil {
		t.Fatalf("update config: %v", err)
	}

	recorder.mu.Lock()
	if len(recorder.starts) != 1 {
		recorder.mu.Unlock()
		t.Fatalf("expected exactly one recorder start, got %d", len(recorder.starts))
	}
	startDir := recorder.starts[0].PartsDir
	recorder.mu.Unlock()
	if filepath.Base(filepath.Dir(startDir)) != "session-1" {
		t.Fatalf("unexpected session dir: %s", startDir)
	}
	if err := manager.SetAutoRecord(context.Background(), false); err != nil {
		t.Fatalf("stop auto record: %v", err)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if status := manager.Status(); status.State == model.ServiceStateDisabled {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	status := manager.Status()
	if status.State != model.ServiceStateDisabled {
		t.Fatalf("expected disabled state, got %s", status.State)
	}
	if recorder.mergeCount == 0 {
		t.Fatal("expected merge to be triggered after stopping recording")
	}
}
