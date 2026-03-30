package model

import "time"

const (
	ServiceStateDisabled  = "disabled"
	ServiceStateIdle      = "idle"
	ServiceStateRecording = "recording"
	ServiceStateStopping  = "stopping"
	ServiceStateMerging   = "merging"
	ServiceStateError     = "error"
)

type AppConfig struct {
	StreamerName        string    `json:"streamer_name"`
	RoomURL             string    `json:"room_url"`
	AutoRecordEnabled   bool      `json:"auto_record_enabled"`
	PollIntervalSeconds int       `json:"poll_interval_seconds"`
	StreamQuality       string    `json:"stream_quality"`
	SegmentMinutes      int       `json:"segment_minutes"`
	SaveSubdir          string    `json:"save_subdir"`
	KeepDays            int       `json:"keep_days"`
	MinFreeGB           int       `json:"min_free_gb"`
	CleanupToGB         int       `json:"cleanup_to_gb"`
	CookiesFile         string    `json:"cookies_file"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func DefaultConfig() AppConfig {
	return AppConfig{
		StreamerName:        "",
		RoomURL:             "",
		AutoRecordEnabled:   false,
		PollIntervalSeconds: 30,
		StreamQuality:       "best",
		SegmentMinutes:      15,
		SaveSubdir:          "/default",
		KeepDays:            7,
		MinFreeGB:           8,
		CleanupToGB:         12,
		CookiesFile:         "",
		UpdatedAt:           time.Now().UTC(),
	}
}

type ConfigApplyInfo struct {
	Field string `json:"field"`
	Mode  string `json:"mode"`
}

func ConfigApplyMatrix() []ConfigApplyInfo {
	return []ConfigApplyInfo{
		{Field: "streamer_name", Mode: "立即生效"},
		{Field: "room_url", Mode: "下一场生效"},
		{Field: "auto_record_enabled", Mode: "立即生效"},
		{Field: "poll_interval_seconds", Mode: "立即生效"},
		{Field: "stream_quality", Mode: "下一场生效"},
		{Field: "segment_minutes", Mode: "下一场生效"},
		{Field: "save_subdir", Mode: "下一场生效"},
		{Field: "keep_days", Mode: "立即生效"},
		{Field: "min_free_gb", Mode: "立即生效"},
		{Field: "cleanup_to_gb", Mode: "立即生效"},
		{Field: "cookies_file", Mode: "下一场生效"},
	}
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type Session struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
}

type RecordSession struct {
	ID             int64      `json:"id"`
	StreamerName   string     `json:"streamer_name"`
	RoomURL        string     `json:"room_url"`
	SaveSubdir     string     `json:"save_subdir"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	FinalFilePath  string     `json:"final_file_path"`
	FileSizeBytes  int64      `json:"file_size_bytes"`
	ErrorMessage   string     `json:"error_message"`
	Quality        string     `json:"quality"`
	SegmentMinutes int        `json:"segment_minutes"`
}

type RecordSegment struct {
	ID              int64
	RecordSessionID int64
	SegmentIndex    int
	StartedAt       time.Time
	EndedAt         *time.Time
	FilePath        string
	Merged          bool
}

type ServiceEvent struct {
	ID        int64
	Level     string
	EventType string
	Message   string
	CreatedAt time.Time
}

type RuntimeStatus struct {
	State             string         `json:"state"`
	Message           string         `json:"message"`
	LastCheckAt       *time.Time     `json:"last_check_at,omitempty"`
	CurrentSessionID  *int64         `json:"current_session_id,omitempty"`
	CurrentSession    *RecordSession `json:"current_session,omitempty"`
	CurrentConfig     AppConfig      `json:"current_config"`
	DiskFreeBytes     uint64         `json:"disk_free_bytes"`
	DiskTotalBytes    uint64         `json:"disk_total_bytes"`
	RecordingRoot     string         `json:"recording_root"`
	AppliedConfigHint []ConfigApplyInfo `json:"applied_config_hint"`
}

