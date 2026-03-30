package env

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr      string
	DBPath          string
	RecordingsRoot  string
	CookiesRoot     string
	SessionTTL      time.Duration
	AdminUsername   string
	AdminPassword   string
	ProbeTimeout    time.Duration
	ProcessStopWait time.Duration
}

func Load() Config {
	return Config{
		ListenAddr:      getEnv("APP_LISTEN_ADDR", ":8080"),
		DBPath:          getEnv("APP_DB_PATH", "/data/db/recorder.db"),
		RecordingsRoot:  getEnv("APP_RECORDINGS_ROOT", "/data/recordings"),
		CookiesRoot:     getEnv("APP_COOKIES_ROOT", "/data/cookies"),
		SessionTTL:      getEnvDurationHours("APP_SESSION_TTL_HOURS", 168),
		AdminUsername:   getEnv("APP_ADMIN_USERNAME", "admin"),
		AdminPassword:   getEnv("APP_ADMIN_PASSWORD", "admin123456"),
		ProbeTimeout:    getEnvDurationSeconds("APP_PROBE_TIMEOUT_SECONDS", 20),
		ProcessStopWait: getEnvDurationSeconds("APP_PROCESS_STOP_WAIT_SECONDS", 8),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvDurationHours(key string, fallback int) time.Duration {
	value := getEnvInt(key, fallback)
	return time.Duration(value) * time.Hour
}

func getEnvDurationSeconds(key string, fallback int) time.Duration {
	value := getEnvInt(key, fallback)
	return time.Duration(value) * time.Second
}

func getEnvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}
