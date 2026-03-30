package recording

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ProbeResult struct {
	Live      bool
	StreamURL string
}

type LiveProbe interface {
	Probe(ctx context.Context, roomURL, quality, cookieHeader string) (ProbeResult, error)
}

type Recorder interface {
	Start(ctx context.Context, opts StartOptions) (Handle, error)
	Merge(ctx context.Context, partsDir, outputFile string) error
}

type StartOptions struct {
	RoomURL        string
	Quality        string
	PartsDir       string
	SegmentMinutes int
	CookieHeader   string
	StartIndex     int
}

type Handle interface {
	Stop(ctx context.Context) error
	Wait() error
}

type CLI struct {
	logger   *slog.Logger
	stopWait time.Duration
}

func NewCLI(logger *slog.Logger, stopWait time.Duration) *CLI {
	return &CLI{logger: logger, stopWait: stopWait}
}

func (c *CLI) Probe(ctx context.Context, roomURL, quality, cookieHeader string) (ProbeResult, error) {
	args := []string{"--stream-url"}
	if cookieHeader != "" {
		args = append(args, "--http-header", "Cookie="+cookieHeader)
	}
	args = append(args, roomURL, quality)
	cmd := exec.CommandContext(ctx, "streamlink", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		message := strings.ToLower(strings.TrimSpace(strings.TrimSpace(stdout.String()) + "\n" + strings.TrimSpace(stderr.String())))
		if isOfflineProbeMessage(message) {
			return ProbeResult{Live: false}, nil
		}
		if strings.TrimSpace(message) == "" {
			return ProbeResult{Live: false}, nil
		}
		return ProbeResult{}, fmt.Errorf("streamlink probe failed: %w: %s", err, strings.TrimSpace(message))
	}
	url := strings.TrimSpace(stdout.String())
	return ProbeResult{Live: url != "", StreamURL: url}, nil
}

func isOfflineProbeMessage(message string) bool {
	return strings.Contains(message, "no playable streams") ||
		strings.Contains(message, "offline") ||
		strings.Contains(message, "not currently live")
}

func (c *CLI) Start(ctx context.Context, opts StartOptions) (Handle, error) {
	if err := os.MkdirAll(opts.PartsDir, 0o755); err != nil {
		return nil, err
	}

	procCtx, cancel := context.WithCancel(ctx)

	streamArgs := []string{"--stdout"}
	if opts.CookieHeader != "" {
		streamArgs = append(streamArgs, "--http-header", "Cookie="+opts.CookieHeader)
	}
	streamArgs = append(streamArgs, opts.RoomURL, opts.Quality)
	streamCmd := exec.CommandContext(procCtx, "streamlink", streamArgs...)

	ffmpegArgs := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", "pipe:0",
		"-map", "0",
		"-c", "copy",
		"-f", "segment",
		"-segment_time", strconv.Itoa(opts.SegmentMinutes * 60),
		"-segment_start_number", strconv.Itoa(opts.StartIndex),
		"-reset_timestamps", "1",
		filepath.Join(opts.PartsDir, "part-%05d.ts"),
	}
	ffmpegCmd := exec.CommandContext(procCtx, "ffmpeg", ffmpegArgs...)

	streamPipe, err := streamCmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	ffmpegCmd.Stdin = streamPipe

	var streamErrBuf, ffmpegErrBuf bytes.Buffer
	streamCmd.Stderr = &streamErrBuf
	ffmpegCmd.Stderr = &ffmpegErrBuf

	if err := ffmpegCmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	if err := streamCmd.Start(); err != nil {
		cancel()
		_ = ffmpegCmd.Process.Kill()
		return nil, err
	}

	handle := &processHandle{
		cancel:    cancel,
		logger:    c.logger,
		stopWait:  c.stopWait,
		streamCmd: streamCmd,
		ffmpegCmd: ffmpegCmd,
		streamErr: &streamErrBuf,
		ffmpegErr: &ffmpegErrBuf,
		done:      make(chan struct{}),
	}
	go handle.wait()
	return handle, nil
}

func (c *CLI) Merge(ctx context.Context, partsDir, outputFile string) error {
	entries, err := os.ReadDir(partsDir)
	if err != nil {
		return err
	}
	var parts []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ts") {
			continue
		}
		parts = append(parts, filepath.Join(partsDir, entry.Name()))
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return errors.New("no ts segments found")
	}

	listFile := filepath.Join(partsDir, "merge.ffconcat")
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString("file '")
		builder.WriteString(strings.ReplaceAll(filepath.ToSlash(part), "'", "'\\''"))
		builder.WriteString("'\n")
	}
	if err := os.WriteFile(listFile, []byte(builder.String()), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputFile), 0o755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "concat", "-safe", "0", "-i", listFile,
		"-c", "copy", "-movflags", "+faststart",
		outputFile,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg merge failed: %w: %s", err, stderr.String())
	}
	return nil
}

type processHandle struct {
	cancel    context.CancelFunc
	logger    *slog.Logger
	stopWait  time.Duration
	streamCmd *exec.Cmd
	ffmpegCmd *exec.Cmd
	streamErr *bytes.Buffer
	ffmpegErr *bytes.Buffer

	done   chan struct{}
	result error
	mu     sync.RWMutex
}

func (p *processHandle) wait() {
	streamErr := p.streamCmd.Wait()
	ffmpegErr := p.ffmpegCmd.Wait()

	var result error
	if streamErr != nil || ffmpegErr != nil {
		message := ""
		if p.streamErr.Len() > 0 {
			message = p.streamErr.String()
		}
		if p.ffmpegErr.Len() > 0 {
			message = strings.TrimSpace(message + "; " + p.ffmpegErr.String())
		}
		if message == "" {
			message = "recording process exited unexpectedly"
		}
		result = fmt.Errorf("%v %v: %s", streamErr, ffmpegErr, message)
	}

	p.mu.Lock()
	p.result = result
	p.mu.Unlock()
	close(p.done)
}

func (p *processHandle) Stop(ctx context.Context) error {
	p.cancel()
	waitCtx, cancel := context.WithTimeout(ctx, p.stopWait)
	defer cancel()

	select {
	case <-p.done:
		return p.Wait()
	case <-waitCtx.Done():
		if p.streamCmd.Process != nil {
			_ = p.streamCmd.Process.Kill()
		}
		if p.ffmpegCmd.Process != nil {
			_ = p.ffmpegCmd.Process.Kill()
		}
		select {
		case <-p.done:
			return p.Wait()
		case <-time.After(2 * time.Second):
			return waitCtx.Err()
		}
	}
}

func (p *processHandle) Wait() error {
	<-p.done
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.result
}
