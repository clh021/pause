package remoteserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"pause/internal/backend/bootstrap"
	"pause/internal/logx"
)

// PrepareScreenshotFunc hides UI that should not appear in remote captures.
type PrepareScreenshotFunc func(ctx context.Context) (restore func(), err error)

// Server exposes a lightweight remote control surface for the local network.
type Server struct {
	cfg              Config
	engine           bootstrap.RuntimeEngine
	screenshots      *ScreenshotService
	beforeScreenshot PrepareScreenshotFunc
	httpServer       *http.Server
	now              func() time.Time
	triggerCooldown  time.Duration
	triggerMu        sync.Mutex
	lastTriggerAt    time.Time
}

// NewServer constructs the remote HTTP server.
func NewServer(cfg Config, engine bootstrap.RuntimeEngine, beforeScreenshot PrepareScreenshotFunc) (*Server, error) {
	if engine == nil {
		return nil, errors.New("runtime engine is required")
	}
	capturer := NewScreenshotCapturer()
	screenshots, err := NewScreenshotService(capturer)
	if err != nil {
		return nil, err
	}
	cfg = cfg.Normalize()
	server := &Server{
		cfg:              cfg,
		engine:           engine,
		screenshots:      screenshots,
		beforeScreenshot: beforeScreenshot,
		now:              time.Now,
		triggerCooldown:  time.Duration(cfg.TriggerCooldownSec) * time.Second,
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

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
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

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	logx.Infof("remote_server.stop addr=%s", s.httpServer.Addr)
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/screenshot", s.handleScreenshot)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/trigger-break", s.handleTriggerBreak)
	mux.HandleFunc("/api/skip-break", s.handleSkipBreak)
	mux.HandleFunc("/api/pause", s.handlePause)
	mux.HandleFunc("/api/resume", s.handleResume)

	return s.withCORS(s.requireAuth(mux))
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
