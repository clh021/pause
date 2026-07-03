package remoteserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"pause/internal/logx"
)

// PrepareScreenshotFunc hides UI that should not appear in remote captures.
type PrepareScreenshotFunc func(ctx context.Context) (restore func(), err error)

// Server exposes a lightweight remote control surface for the local network.
type Server struct {
	cfg              Config
	services         Services
	screenshots      *ScreenshotService
	beforeScreenshot PrepareScreenshotFunc
	httpServer       *http.Server
	now              func() time.Time
	triggerCooldown  time.Duration
	triggerMu        sync.Mutex
	lastTriggerAt    time.Time
	staticFileServer http.Handler

	activity *ActivityRecorder
}

// NewServer constructs the remote HTTP server.
func NewServer(cfg Config, services Services, beforeScreenshot PrepareScreenshotFunc) (*Server, error) {
	if services.Engine == nil {
		return nil, errors.New("runtime engine is required")
	}
	capturer := NewScreenshotCapturer()
	ss, err := NewScreenshotService(capturer)
	if err != nil {
		return nil, err
	}
	cfg = cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	server := &Server{
		cfg:              cfg,
		services:         services,
		screenshots:      ss,
		beforeScreenshot: beforeScreenshot,
		now:              time.Now,
		triggerCooldown:  time.Duration(cfg.TriggerCooldownSec) * time.Second,
		staticFileServer: staticFileHandler(),
	}
	server.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.BindAddress, cfg.Port),
		Handler: server.routes(),
	}
	return server, nil
}

// Start launches the remote server until ctx is canceled.
func (s *Server) Start(ctx context.Context) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}

	// Start the activity recorder (polls every 10s).
	if rec, err := NewActivityRecorder(s.services.Engine, s.screenshots, true); err == nil {
		s.activity = rec
		go s.activityPollLoop(ctx)
	} else {
		logx.Warnf("remote_server.activity_start_err err=%v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		if s.activity != nil {
			s.activity.Close()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Shutdown(shutdownCtx); err != nil {
			logx.Warnf("remote_server.shutdown_err err=%v", err)
		}
	}()

	go func() {
		logx.Infof("remote_server.start addr=%s token_enabled=%t", s.httpServer.Addr, s.cfg.Token != "")
		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(150 * time.Millisecond):
		return nil
	}
}

func (s *Server) activityPollLoop(ctx context.Context) {
	ticker := time.NewTicker(activityPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.activity.Tick(ctx)
		}
	}
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	logx.Infof("remote_server.stop addr=%s", s.httpServer.Addr)
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) routes() http.Handler {
	protected := http.NewServeMux()
	protected.HandleFunc("/screenshot", s.handleScreenshot)
	protected.HandleFunc("/api/status", s.handleStatus)
	protected.HandleFunc("/api/trigger-break", s.handleTriggerBreak)
	protected.HandleFunc("/api/skip-break", s.handleSkipBreak)
	protected.HandleFunc("/api/postpone-break", s.handlePostponeBreak)
	protected.HandleFunc("/api/pause", s.handlePause)
	protected.HandleFunc("/api/resume", s.handleResume)
	protected.HandleFunc("/api/reminders", s.handleListReminders)
	protected.HandleFunc("/api/reminders/create", s.handleCreateReminder)
	protected.HandleFunc("/api/reminders/update/", s.handleUpdateReminder)
	protected.HandleFunc("/api/reminders/delete/", s.handleDeleteReminder)
	protected.HandleFunc("/api/reminders/pause/", s.handlePauseReminder)
	protected.HandleFunc("/api/reminders/resume/", s.handleResumeReminder)
	protected.HandleFunc("/api/settings", s.handleGetSettings)
	protected.HandleFunc("/api/settings/update", s.handleUpdateSettings)
	protected.HandleFunc("/api/settings/launch-at-login", s.handleGetLaunchAtLogin)
	protected.HandleFunc("/api/settings/launch-at-login/set", s.handleSetLaunchAtLogin)
	protected.HandleFunc("/api/settings/auto-screenshot", s.handleAutoScreenshotSetting)
	protected.HandleFunc("/api/analytics/weekly", s.handleWeeklyStats)
	protected.HandleFunc("/api/analytics/summary", s.handleAnalyticsSummary)
	protected.HandleFunc("/api/analytics/trend", s.handleAnalyticsTrend)
	protected.HandleFunc("/api/analytics/distribution", s.handleBreakTypeDistribution)
	protected.HandleFunc("/api/notification/capability", s.handleNotificationCapability)
	protected.HandleFunc("/api/notification/request", s.handleRequestNotificationPermission)
	protected.HandleFunc("/api/notification/open-settings", s.handleOpenNotificationSettings)
	protected.HandleFunc("/api/force-unlock", s.handleForceUnlock)
	protected.HandleFunc("/api/force-break", s.handleForceBreak)
	protected.HandleFunc("/api/quit", s.handleQuit)
	protected.HandleFunc("/api/runtime", s.handleRuntimeState)
	protected.HandleFunc("/api/activity", s.handleGetActivity)
	protected.HandleFunc("/api/screenshots", s.handleScreenshotList)
	protectedHandler := s.withCORS(s.requireAuth(protected))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/auth/session":
			s.withCORS(http.HandlerFunc(s.handleAuthSession)).ServeHTTP(w, r)
		case r.URL.Path == "/screenshot":
			protectedHandler.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/shots/"):
			s.withCORS(s.requireAuth(http.HandlerFunc(s.handleServeShot))).ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/"):
			protectedHandler.ServeHTTP(w, r)
		case s.staticFileServer != nil:
			s.staticFileServer.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		allowedOrigin := s.allowedOrigin(origin, r.Host)
		if origin != "" && allowedOrigin == "" {
			if r.Method == http.MethodOptions {
				writeJSONError(w, http.StatusForbidden, "origin not allowed")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowedOrigin(origin string, host string) string {
	if origin == "" || host == "" {
		return ""
	}
	if origin == "http://"+host || origin == "https://"+host {
		return origin
	}
	switch origin {
	case "http://wails.localhost", "https://wails.localhost", "wails://wails":
		return origin
	}
	return ""
}

// testActivityDir is overridable in tests.
var testActivityDir = defaultActivityDir

func defaultActivityDir() string {
	return filepath.Join(homeDir(), ".pause", "activity")
}

var testScreenshotDir = defaultScreenshotDir

func (s *Server) screenshotDir() string {
	return testScreenshotDir()
}

func screenshotsBaseDir() string {
	return testScreenshotDir()
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func defaultScreenshotDir() string {
	return filepath.Join(homeDir(), ".pause", "screenshots")
}
