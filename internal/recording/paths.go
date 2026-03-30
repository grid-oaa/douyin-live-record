package recording

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"douyin-live-record/internal/model"
)

func NormalizeSaveSubdir(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		trimmed = "/default"
	}
	for _, part := range strings.Split(strings.ReplaceAll(trimmed, "\\", "/"), "/") {
		if part == ".." {
			return "", errors.New("save_subdir contains invalid traversal path")
		}
	}
	cleaned := filepath.ToSlash(filepath.Clean("/" + strings.TrimPrefix(trimmed, "/")))
	if cleaned == "/." {
		cleaned = "/default"
	}
	if strings.Contains(cleaned, "..") {
		return "", errors.New("save_subdir contains invalid traversal path")
	}
	return cleaned, nil
}

func ResolveSaveDir(root, subdir string) (string, error) {
	normalized, err := NormalizeSaveSubdir(subdir)
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(normalized, "/")))
	return resolved, nil
}

func ResolveCookieFile(root, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	cleaned := filepath.Clean(strings.TrimPrefix(trimmed, "/"))
	if cleaned == "." || strings.Contains(cleaned, "..") {
		return "", errors.New("cookies_file must stay within cookies root")
	}
	return filepath.Join(root, cleaned), nil
}

func CollectSegments(sessionID int64, partsDir string, sessionStart time.Time) ([]model.RecordSegment, error) {
	entries, err := os.ReadDir(partsDir)
	if err != nil {
		return nil, err
	}
	type item struct {
		name string
		info os.FileInfo
	}
	var files []item
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ts") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, item{name: entry.Name(), info: info})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	segments := make([]model.RecordSegment, 0, len(files))
	for idx, file := range files {
		ended := file.info.ModTime().UTC()
		started := sessionStart
		if idx > 0 {
			started = files[idx-1].info.ModTime().UTC()
		}
		endedCopy := ended
		segments = append(segments, model.RecordSegment{
			RecordSessionID: sessionID,
			SegmentIndex:    idx,
			StartedAt:       started,
			EndedAt:         &endedCopy,
			FilePath:        filepath.Join(partsDir, file.name),
			Merged:          false,
		})
	}
	return segments, nil
}
