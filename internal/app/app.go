package app

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"douyin-live-record/internal/auth"
	"douyin-live-record/internal/env"
	"douyin-live-record/internal/model"
	"douyin-live-record/internal/recording"
	"douyin-live-record/internal/storage"
)

const routePrefix = "/douyin-live"

type App struct {
	cfg       env.Config
	logger    *slog.Logger
	store     *storage.Store
	auth      *auth.Service
	manager   *recording.Manager
	mux       *http.ServeMux
	templates *template.Template
}

type pageData struct {
	Title       string
	CurrentPath string
	Config      model.AppConfig
	Status      model.RuntimeStatus
	History     []model.RecordSession
	Events      []model.ServiceEvent
	ApplyInfo   []model.ConfigApplyInfo
	Error       string
}

func New(cfg env.Config, logger *slog.Logger) (*App, error) {
	for _, dir := range []string{cfg.RecordingsRoot, cfg.CookiesRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	store, err := storage.New(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	authService := auth.NewService(store, cfg.SessionTTL)
	if err := authService.EnsureAdmin(context.Background(), cfg.AdminUsername, cfg.AdminPassword); err != nil {
		store.Close()
		return nil, err
	}

	cli := recording.NewCLI(logger, cfg.ProcessStopWait)
	manager, err := recording.NewManager(store, logger, cli, cli, cfg.RecordingsRoot, cfg.CookiesRoot, cfg.ProbeTimeout)
	if err != nil {
		store.Close()
		return nil, err
	}
	manager.Start()

	tmpl, err := loadTemplates()
	if err != nil {
		store.Close()
		manager.Stop()
		return nil, err
	}

	app := &App{
		cfg:       cfg,
		logger:    logger,
		store:     store,
		auth:      authService,
		manager:   manager,
		mux:       http.NewServeMux(),
		templates: tmpl,
	}
	app.routes()
	return app, nil
}

func (a *App) Router() http.Handler {
	return a.mux
}

func (a *App) Stop() {
	a.manager.Stop()
}

func (a *App) Close() {
	_ = a.store.Close()
}

func (a *App) routes() {
	a.mux.HandleFunc("/", a.handleRoot)
	a.mux.HandleFunc(routePrefix, a.handleIndex)
	a.mux.HandleFunc(routePrefix+"/{$}", a.handleIndex)
	a.mux.HandleFunc(routePath("/login"), a.handleLoginPage)
	a.mux.HandleFunc(routePath("/healthz"), a.handleHealthz)
	a.mux.HandleFunc(routePath("/assets/app.css"), a.handleCSS)
	a.mux.HandleFunc(routePath("/assets/app.js"), a.handleJS)

	a.mux.HandleFunc(routePath("/api/login"), a.handleAPILogin)
	a.mux.HandleFunc(routePath("/api/logout"), a.requireAPIAuth(a.handleAPILogout))
	a.mux.HandleFunc(routePath("/api/config"), a.requireAPIAuth(a.handleAPIConfig))
	a.mux.HandleFunc(routePath("/api/status"), a.requireAPIAuth(a.handleAPIStatus))
	a.mux.HandleFunc(routePath("/api/history"), a.requireAPIAuth(a.handleAPIHistory))
	a.mux.HandleFunc(routePath("/api/recording/start"), a.requireAPIAuth(a.handleAPIStartRecording))
	a.mux.HandleFunc(routePath("/api/recording/stop"), a.requireAPIAuth(a.handleAPIStopRecording))
	a.mux.HandleFunc(routePath("/api/recording/rebuild-latest"), a.requireAPIAuth(a.handleAPIRebuildLatest))

	a.mux.HandleFunc(routePath("/admin/config"), a.requirePageAuth(a.handleConfigPage))
	a.mux.HandleFunc(routePath("/admin/status"), a.requirePageAuth(a.handleStatusPage))
	a.mux.HandleFunc(routePath("/admin/history"), a.requirePageAuth(a.handleHistoryPage))
}

func (a *App) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, routePath("/"), http.StatusFound)
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != routePrefix && r.URL.Path != routePath("/") {
		http.NotFound(w, r)
		return
	}
	if _, ok := a.sessionToken(r); ok {
		http.Redirect(w, r, routePath("/admin/status"), http.StatusFound)
		return
	}
	http.Redirect(w, r, routePath("/login"), http.StatusFound)
}

func (a *App) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != routePath("/login") {
		http.NotFound(w, r)
		return
	}
	if _, ok := a.sessionToken(r); ok {
		http.Redirect(w, r, routePath("/admin/status"), http.StatusFound)
		return
	}
	a.render(w, "login", pageData{Title: "登录"})
}

func (a *App) handleConfigPage(w http.ResponseWriter, r *http.Request) {
	cfg := a.manager.CurrentConfig()
	status := a.manager.Status()
	a.render(w, "config", pageData{
		Title:       "录制配置",
		CurrentPath: r.URL.Path,
		Config:      cfg,
		Status:      status,
		ApplyInfo:   model.ConfigApplyMatrix(),
	})
}

func (a *App) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	status := a.manager.Status()
	events, _ := a.store.ListEvents(r.Context(), 20)
	a.render(w, "status", pageData{
		Title:       "当前状态",
		CurrentPath: r.URL.Path,
		Status:      status,
		Events:      events,
	})
}

func (a *App) handleHistoryPage(w http.ResponseWriter, r *http.Request) {
	history, _ := a.store.ListRecordSessions(r.Context(), 50)
	a.render(w, "history", pageData{
		Title:       "录制历史",
		CurrentPath: r.URL.Path,
		History:     history,
	})
}

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	_, err := a.store.LoadConfig(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	token, err := a.auth.Login(r.Context(), strings.TrimSpace(payload.Username), payload.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	a.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAPILogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token, _ := a.sessionToken(r)
	_ = a.auth.Logout(r.Context(), token)
	a.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"config":      a.manager.CurrentConfig(),
			"apply_info":  model.ConfigApplyMatrix(),
			"status":      a.manager.Status(),
			"cookiesRoot": a.cfg.CookiesRoot,
		})
	case http.MethodPut:
		var payload model.AppConfig
		if err := decodeJSON(r, &payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		saved, err := a.manager.UpdateConfig(r.Context(), sanitizeConfig(payload))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"config": saved, "apply_info": model.ConfigApplyMatrix()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	events, _ := a.store.ListEvents(r.Context(), 20)
	writeJSON(w, http.StatusOK, map[string]any{
		"status": a.manager.Status(),
		"events": events,
	})
}

func (a *App) handleAPIHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	history, err := a.store.ListRecordSessions(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history})
}

func (a *App) handleAPIStartRecording(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.manager.SetAutoRecord(r.Context(), true); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAPIStopRecording(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.manager.SetAutoRecord(r.Context(), false); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleAPIRebuildLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.manager.RebuildLatest(r.Context()); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) requirePageAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := a.sessionToken(r)
		if !ok {
			http.Redirect(w, r, routePath("/login"), http.StatusFound)
			return
		}
		if _, err := a.auth.Authenticate(r.Context(), token); err != nil {
			a.clearSessionCookie(w)
			http.Redirect(w, r, routePath("/login"), http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (a *App) requireAPIAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := a.sessionToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		if _, err := a.auth.Authenticate(r.Context(), token); err != nil {
			a.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		next(w, r)
	}
}

func (a *App) sessionToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("dlr_session")
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func (a *App) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "dlr_session",
		Value:    token,
		Path:     routePrefix,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(a.cfg.SessionTTL.Seconds()),
	})
}

func (a *App) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "dlr_session",
		Value:    "",
		Path:     routePrefix,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) render(w http.ResponseWriter, name string, data pageData) {
	if err := a.templates.ExecuteTemplate(w, "base", map[string]any{
		"Page": name,
		"Data": data,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		return fmt.Errorf("invalid json body: %w", err)
	}
	return nil
}

func sanitizeConfig(cfg model.AppConfig) model.AppConfig {
	cfg.StreamerName = strings.TrimSpace(cfg.StreamerName)
	cfg.RoomURL = strings.TrimSpace(cfg.RoomURL)
	cfg.StreamQuality = strings.TrimSpace(cfg.StreamQuality)
	cfg.SaveSubdir = strings.TrimSpace(cfg.SaveSubdir)
	cfg.CookiesFile = strings.TrimSpace(cfg.CookiesFile)
	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = 30
	}
	if cfg.StreamQuality == "" {
		cfg.StreamQuality = "best"
	}
	if cfg.SegmentMinutes <= 0 {
		cfg.SegmentMinutes = 15
	}
	if cfg.KeepDays <= 0 {
		cfg.KeepDays = 7
	}
	if cfg.MinFreeGB <= 0 {
		cfg.MinFreeGB = 8
	}
	if cfg.CleanupToGB <= 0 {
		cfg.CleanupToGB = 12
	}
	return cfg
}

//go:embed dummy.txt
var embeddedDummy embed.FS

func loadTemplates() (*template.Template, error) {
	funcMap := template.FuncMap{
		"basePath": func() string {
			return routePrefix
		},
		"appPath": func(path string) string {
			return routePath(path)
		},
		"fmtTime": func(value *time.Time) string {
			if value == nil {
				return "-"
			}
			return value.Local().Format("2006-01-02 15:04:05")
		},
		"fmtTimeValue": func(value time.Time) string {
			if value.IsZero() {
				return "-"
			}
			return value.Local().Format("2006-01-02 15:04:05")
		},
		"fmtBytes": func(value int64) string {
			if value <= 0 {
				return "0 B"
			}
			const unit = 1024
			sizes := []string{"B", "KB", "MB", "GB", "TB"}
			fv := float64(value)
			idx := 0
			for fv >= unit && idx < len(sizes)-1 {
				fv /= unit
				idx++
			}
			return fmt.Sprintf("%.2f %s", fv, sizes[idx])
		},
		"fmtBytesU": func(value uint64) string {
			return fmt.Sprintf("%.2f GB", float64(value)/1024/1024/1024)
		},
		"statusCategory": func(state string) string {
			switch state {
			case model.ServiceStateRecording, model.ServiceStateStopping, model.ServiceStateMerging:
				return "正在录制"
			case model.ServiceStateError:
				return "探测异常"
			default:
				return "未开播"
			}
		},
	}
	return template.New("base").Funcs(funcMap).Parse(baseTemplate + loginTemplate + configTemplate + statusTemplate + historyTemplate)
}

func routePath(path string) string {
	if path == "" || path == "/" {
		return routePrefix + "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return routePrefix + path
}
