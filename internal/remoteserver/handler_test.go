package remoteserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pause/internal/backend/bootstrap"
	settingsdomain "pause/internal/backend/domain/settings"
	"pause/internal/backend/runtime/state"
)

type fakeEngine struct {
	runtimeState state.RuntimeState
	pauseState   state.RuntimeState
	resumeState  state.RuntimeState
	skipState    state.RuntimeState
	startState   state.RuntimeState
	skipErr      error
	startErr     error
}

func (f *fakeEngine) Start(context.Context)                        {}
func (f *fakeEngine) Stop()                                        {}
func (f *fakeEngine) GetSettings() settingsdomain.Settings         { return settingsdomain.DefaultSettings() }
func (f *fakeEngine) GetRuntimeState(time.Time) state.RuntimeState { return f.runtimeState }
func (f *fakeEngine) Pause(time.Time) state.RuntimeState           { return f.pauseState }
func (f *fakeEngine) Resume(time.Time) state.RuntimeState          { return f.resumeState }
func (f *fakeEngine) PauseReminder(int64, time.Time) (state.RuntimeState, error) {
	return state.RuntimeState{}, nil
}
func (f *fakeEngine) ResumeReminder(int64, time.Time) (state.RuntimeState, error) {
	return state.RuntimeState{}, nil
}
func (f *fakeEngine) SkipCurrentBreak(time.Time, bootstrap.SkipMode) (state.RuntimeState, error) {
	return f.skipState, f.skipErr
}
func (f *fakeEngine) PostponeCurrentBreak(time.Time) (state.RuntimeState, error) {
	return state.RuntimeState{}, nil
}
func (f *fakeEngine) StartBreakNow(time.Time) (state.RuntimeState, error) {
	return f.startState, f.startErr
}
func (f *fakeEngine) StartBreakNowForReason(int64, time.Time) (state.RuntimeState, error) {
	return state.RuntimeState{}, nil
}

func TestStatusEndpointReturnsRestingState(t *testing.T) {
	server := newTestServer(t, &fakeEngine{
		runtimeState: state.RuntimeState{
			GlobalEnabled: true,
			CurrentSession: &state.BreakSessionView{
				Status:       "resting",
				StartedAt:    time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
				EndsAt:       time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC),
				RemainingSec: 120,
			},
			Reminders: []state.ReminderRuntime{{ID: 1, Name: "Test", Enabled: true}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if resp.Status != "resting" || resp.RemainingSec != 120 || resp.TotalBreakSec != 300 {
		t.Fatalf("unexpected response %+v", resp)
	}
}

func TestStatusEndpointReturnsPausedWhenGlobalDisabled(t *testing.T) {
	server := newTestServer(t, &fakeEngine{
		runtimeState: state.RuntimeState{GlobalEnabled: false},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if resp.Status != "paused" {
		t.Fatalf("expected paused status, got %+v", resp)
	}
}

func TestAuthMiddlewareRejectsMissingToken(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	server.cfg.Token = "secret"

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestTriggerBreakAppliesCooldown(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	server := newTestServer(t, &fakeEngine{
		startState: state.RuntimeState{GlobalEnabled: true},
	})
	server.now = func() time.Time { return now }
	server.triggerCooldown = 60 * time.Second

	firstReq := httptest.NewRequest(http.MethodPost, "/api/trigger-break", bytes.NewBufferString(`{"reason":"remote","breakSec":300}`))
	firstRec := httptest.NewRecorder()
	server.routes().ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/trigger-break", bytes.NewBufferString(`{"reason":"remote","breakSec":300}`))
	secondRec := httptest.NewRecorder()
	server.routes().ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("expected second request 409, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
}

func TestScreenshotEndpointRunsPreparationAndReturnsPNG(t *testing.T) {
	prepared := false
	restored := false
	server := newTestServer(t, &fakeEngine{})
	server.beforeScreenshot = func(context.Context) (func(), error) {
		prepared = true
		return func() { restored = true }, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/screenshot?t=123", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("expected image/png, got %q", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != "test-png" {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}
	if !prepared || !restored {
		t.Fatalf("expected prepare/restore to run, prepared=%t restored=%t", prepared, restored)
	}
}

func TestTriggerBreakMapsEngineConflictTo409(t *testing.T) {
	server := newTestServer(t, &fakeEngine{
		startErr: errors.New("break already active"),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/trigger-break", bytes.NewBufferString(`{"reason":"remote"}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func newTestServer(t *testing.T, engine *fakeEngine) *Server {
	t.Helper()
	return &Server{
		cfg:             DefaultConfig(),
		engine:          engine,
		screenshots:     &ScreenshotService{capturer: fakeScreenshotCapturer{png: []byte("test-png")}, dir: t.TempDir(), now: time.Now},
		now:             time.Now,
		triggerCooldown: 60 * time.Second,
	}
}
