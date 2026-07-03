package remoteserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pause/internal/backend/bootstrap"
	analyticsdomain "pause/internal/backend/domain/analytics"
	reminderdomain "pause/internal/backend/domain/reminder"
	settingsdomain "pause/internal/backend/domain/settings"
	"pause/internal/backend/ports"
	"pause/internal/backend/runtime/state"
)

type fakeEngine struct {
	runtimeState      state.RuntimeState
	pauseState        state.RuntimeState
	resumeState       state.RuntimeState
	skipState         state.RuntimeState
	startState        state.RuntimeState
	skipErr           error
	startErr          error
	pauseReminderErr  error
	resumeReminderErr error
}

func (f *fakeEngine) Start(context.Context)                        {}
func (f *fakeEngine) Stop()                                        {}
func (f *fakeEngine) GetSettings() settingsdomain.Settings         { return settingsdomain.DefaultSettings() }
func (f *fakeEngine) GetRuntimeState(time.Time) state.RuntimeState { return f.runtimeState }
func (f *fakeEngine) Pause(time.Time) state.RuntimeState           { return f.pauseState }
func (f *fakeEngine) Resume(time.Time) state.RuntimeState          { return f.resumeState }
func (f *fakeEngine) PauseReminder(int64, time.Time) (state.RuntimeState, error) {
	return state.RuntimeState{}, f.pauseReminderErr
}
func (f *fakeEngine) ResumeReminder(int64, time.Time) (state.RuntimeState, error) {
	return state.RuntimeState{}, f.resumeReminderErr
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

func TestHandleAuthSessionSetsCookie(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	server.cfg.Token = "secret"

	req := httptest.NewRequest(http.MethodPost, "/api/auth/session", bytes.NewBufferString(`{"token":"secret"}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != remoteAuthCookieName {
		t.Fatalf("expected auth cookie, got %+v", cookies)
	}
}

func TestWithCORSAllowsWailsOrigin(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	server.cfg.Token = "secret"

	req := httptest.NewRequest(http.MethodOptions, "/api/runtime", nil)
	req.Header.Set("Origin", "http://wails.localhost")
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://wails.localhost" {
		t.Fatalf("expected Wails origin to be allowed, got %q", got)
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
		services:        Services{Engine: engine},
		screenshots:     &ScreenshotService{capturer: fakeScreenshotCapturer{png: []byte("test-png")}, dir: t.TempDir(), now: time.Now},
		now:             time.Now,
		triggerCooldown: 60 * time.Second,
	}
}

// ---- Fake service implementations ----

type fakeReminderService struct {
	reminders []reminderdomain.Reminder
	createErr error
	updateErr error
	deleteErr error
}

func (f *fakeReminderService) List(ctx context.Context) ([]reminderdomain.Reminder, error) {
	return f.reminders, nil
}

func (f *fakeReminderService) Create(ctx context.Context, _ reminderdomain.CreateInput) ([]reminderdomain.Reminder, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.reminders, nil
}

func (f *fakeReminderService) Update(ctx context.Context, _ reminderdomain.Patch) ([]reminderdomain.Reminder, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.reminders, nil
}

func (f *fakeReminderService) Delete(ctx context.Context, _ int64) ([]reminderdomain.Reminder, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return f.reminders, nil
}

type fakeAnalyticsService struct {
	weeklyStats analyticsdomain.WeeklyStats
	summary     analyticsdomain.Summary
	trend       analyticsdomain.Trend
	dist        analyticsdomain.BreakTypeDistribution
	err         error
}

func (f *fakeAnalyticsService) GetWeeklyStats(_ context.Context, _, _ int64) (analyticsdomain.WeeklyStats, error) {
	return f.weeklyStats, f.err
}

func (f *fakeAnalyticsService) GetSummary(_ context.Context, _, _ int64) (analyticsdomain.Summary, error) {
	return f.summary, f.err
}

func (f *fakeAnalyticsService) GetTrendByDay(_ context.Context, _, _ int64) (analyticsdomain.Trend, error) {
	return f.trend, f.err
}

func (f *fakeAnalyticsService) GetBreakTypeDistribution(_ context.Context, _, _ int64) (analyticsdomain.BreakTypeDistribution, error) {
	return f.dist, f.err
}

type fakeSettingsService struct {
	settings      settingsdomain.Settings
	launchAtLogin bool
	err           error
}

func (f *fakeSettingsService) Get(_ context.Context) settingsdomain.Settings {
	return f.settings
}

func (f *fakeSettingsService) Update(_ context.Context, _ settingsdomain.SettingsPatch) (settingsdomain.Settings, error) {
	if f.err != nil {
		return settingsdomain.Settings{}, f.err
	}
	return f.settings, nil
}

func (f *fakeSettingsService) GetLaunchAtLogin(_ context.Context) (bool, error) {
	return f.launchAtLogin, f.err
}

func (f *fakeSettingsService) SetLaunchAtLogin(_ context.Context, enabled bool) (bool, error) {
	return enabled, f.err
}

type fakeCapabilityProvider struct {
	cap ports.NotificationCapability
	err error
}

func (f *fakeCapabilityProvider) GetNotificationCapability() ports.NotificationCapability {
	return f.cap
}

func (f *fakeCapabilityProvider) RequestNotificationPermission() (ports.NotificationCapability, error) {
	return f.cap, f.err
}

func (f *fakeCapabilityProvider) OpenNotificationSettings() error {
	return f.err
}

func newTestServerWithServices(t *testing.T, engine *fakeEngine, svc Services) *Server {
	t.Helper()
	if svc.Engine == nil {
		svc.Engine = engine
	}
	if svc.ReminderService == nil {
		svc.ReminderService = &fakeReminderService{}
	}
	if svc.AnalyticsService == nil {
		svc.AnalyticsService = &fakeAnalyticsService{}
	}
	if svc.SettingsService == nil {
		svc.SettingsService = &fakeSettingsService{}
	}
	if svc.NotificationCapabilityProvider == nil {
		svc.NotificationCapabilityProvider = &fakeCapabilityProvider{}
	}
	return &Server{
		cfg:             DefaultConfig(),
		services:        svc,
		screenshots:     &ScreenshotService{capturer: fakeScreenshotCapturer{png: []byte("test-png")}, dir: t.TempDir(), now: time.Now},
		now:             time.Now,
		triggerCooldown: 60 * time.Second,
	}
}

// ---- Reminder tests ----

func TestHandleListReminders_Success(t *testing.T) {
	svc := Services{
		ReminderService: &fakeReminderService{
			reminders: []reminderdomain.Reminder{
				{ID: 1, Name: "Test", Enabled: true, IntervalSec: 3600, BreakSec: 300, ReminderType: "rest"},
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/reminders", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var reminders []ReminderConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &reminders); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if len(reminders) != 1 || reminders[0].ID != 1 || reminders[0].Name != "Test" {
		t.Fatalf("unexpected reminders %+v", reminders)
	}
}

func TestHandleCreateReminder_Success(t *testing.T) {
	svc := Services{
		ReminderService: &fakeReminderService{
			reminders: []reminderdomain.Reminder{
				{ID: 1, Name: "NewReminder", Enabled: true, IntervalSec: 1800, BreakSec: 120, ReminderType: "rest"},
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	body := `{"name":"NewReminder","intervalSec":1800,"breakSec":120,"reminderType":"rest"}`
	req := httptest.NewRequest(http.MethodPost, "/api/reminders/create", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var reminders []ReminderConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &reminders); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if len(reminders) != 1 || reminders[0].Name != "NewReminder" {
		t.Fatalf("unexpected reminders %+v", reminders)
	}
}

func TestHandleCreateReminder_BadRequest(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})

	req := httptest.NewRequest(http.MethodPost, "/api/reminders/create", bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUpdateReminder_Success(t *testing.T) {
	svc := Services{
		ReminderService: &fakeReminderService{
			reminders: []reminderdomain.Reminder{
				{ID: 1, Name: "Updated", Enabled: true, IntervalSec: 3600, BreakSec: 300, ReminderType: "rest"},
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	body := `{"name":"Updated","intervalSec":3600,"breakSec":300}`
	req := httptest.NewRequest(http.MethodPut, "/api/reminders/update/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var reminders []ReminderConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &reminders); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if len(reminders) != 1 || reminders[0].Name != "Updated" {
		t.Fatalf("unexpected reminders %+v", reminders)
	}
}

func TestHandleUpdateReminder_InvalidID(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})

	req := httptest.NewRequest(http.MethodPut, "/api/reminders/update/abc", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleDeleteReminder_Success(t *testing.T) {
	svc := Services{
		ReminderService: &fakeReminderService{
			reminders: []reminderdomain.Reminder{
				{ID: 2, Name: "AfterDelete", Enabled: true, IntervalSec: 3600, BreakSec: 300, ReminderType: "rest"},
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/reminders/delete/1", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var reminders []ReminderConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &reminders); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if len(reminders) != 1 || reminders[0].ID != 2 {
		t.Fatalf("unexpected reminders %+v", reminders)
	}
}

func TestHandleDeleteReminder_InvalidID(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})

	req := httptest.NewRequest(http.MethodDelete, "/api/reminders/delete/abc", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePauseReminder_Success(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{
		runtimeState: state.RuntimeState{GlobalEnabled: true},
	}, Services{})

	req := httptest.NewRequest(http.MethodPost, "/api/reminders/pause/1", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
}

func TestHandlePauseReminder_InvalidID(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})

	req := httptest.NewRequest(http.MethodPost, "/api/reminders/pause/abc", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleResumeReminder_Success(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{
		runtimeState: state.RuntimeState{GlobalEnabled: true},
	}, Services{})

	req := httptest.NewRequest(http.MethodPost, "/api/reminders/resume/1", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
}

func TestHandleResumeReminder_InvalidID(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})

	req := httptest.NewRequest(http.MethodPost, "/api/reminders/resume/abc", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---- Settings tests ----

func TestHandleGetSettings_Success(t *testing.T) {
	svc := Services{
		SettingsService: &fakeSettingsService{
			settings: settingsdomain.DefaultSettings(),
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var s Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if !s.Sound.Enabled {
		t.Fatalf("expected sound enabled, got %+v", s)
	}
}

func TestHandleUpdateSettings_Success(t *testing.T) {
	svc := Services{
		SettingsService: &fakeSettingsService{
			settings: settingsdomain.DefaultSettings(),
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	body := `{"sound":{"enabled":false}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/settings/update", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var s Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
}

func TestHandleUpdateSettings_BadRequest(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})

	req := httptest.NewRequest(http.MethodPatch, "/api/settings/update", bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleGetLaunchAtLogin(t *testing.T) {
	svc := Services{
		SettingsService: &fakeSettingsService{
			launchAtLogin: true,
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/launch-at-login", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if !resp["enabled"] {
		t.Fatalf("expected enabled=true, got %+v", resp)
	}
}

func TestHandleGetLaunchAtLogin_Error(t *testing.T) {
	svc := Services{
		SettingsService: &fakeSettingsService{
			err: errors.New("some error"),
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/launch-at-login", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleSetLaunchAtLogin(t *testing.T) {
	svc := Services{
		SettingsService: &fakeSettingsService{},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	body := `{"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings/launch-at-login/set", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if !resp["enabled"] {
		t.Fatalf("expected enabled=true, got %+v", resp)
	}
}

func TestHandleSetLaunchAtLogin_BadRequest(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})

	req := httptest.NewRequest(http.MethodPost, "/api/settings/launch-at-login/set", bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---- Analytics tests ----

func TestHandleWeeklyStats_Success(t *testing.T) {
	svc := Services{
		AnalyticsService: &fakeAnalyticsService{
			weeklyStats: analyticsdomain.WeeklyStats{
				FromSec: 1000,
				ToSec:   2000,
				Reminders: []analyticsdomain.ReminderStat{
					{ReminderID: 1, ReminderName: "Test", TriggeredCount: 5, CompletedCount: 3},
				},
				Summary: analyticsdomain.SummaryStats{
					TotalSessions: 5, TotalCompleted: 3,
				},
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/analytics/weekly?fromSec=1000&toSec=2000", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var stats AnalyticsWeeklyStats
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if stats.FromSec != 1000 || stats.ToSec != 2000 || len(stats.Reminders) != 1 {
		t.Fatalf("unexpected stats %+v", stats)
	}
}

func TestHandleWeeklyStats_MissingParams(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})

	req := httptest.NewRequest(http.MethodGet, "/api/analytics/weekly", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAnalyticsSummary_Success(t *testing.T) {
	svc := Services{
		AnalyticsService: &fakeAnalyticsService{
			summary: analyticsdomain.Summary{
				FromSec: 1000, ToSec: 2000,
				TotalSessions: 10, TotalCompleted: 7, TotalSkipped: 3,
				CompletionRate: 0.7, SkipRate: 0.3,
				TotalActualBreakSec: 3600, AvgActualBreakSec: 360,
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/analytics/summary?fromSec=1000&toSec=2000", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var summary AnalyticsSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if summary.TotalSessions != 10 || summary.TotalCompleted != 7 {
		t.Fatalf("unexpected summary %+v", summary)
	}
}

func TestHandleAnalyticsTrend_Success(t *testing.T) {
	svc := Services{
		AnalyticsService: &fakeAnalyticsService{
			trend: analyticsdomain.Trend{
				FromSec: 1000, ToSec: 2000,
				Points: []analyticsdomain.TrendPoint{
					{Day: "2026-01-01", TotalSessions: 5, TotalCompleted: 4},
				},
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/analytics/trend?fromSec=1000&toSec=2000", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var trend AnalyticsTrend
	if err := json.Unmarshal(rec.Body.Bytes(), &trend); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if len(trend.Points) != 1 || trend.Points[0].Day != "2026-01-01" {
		t.Fatalf("unexpected trend %+v", trend)
	}
}

func TestHandleBreakTypeDistribution_Success(t *testing.T) {
	svc := Services{
		AnalyticsService: &fakeAnalyticsService{
			dist: analyticsdomain.BreakTypeDistribution{
				FromSec: 1000, ToSec: 2000, TotalTriggered: 10,
				Items: []analyticsdomain.BreakTypeDistributionItem{
					{ReminderID: 1, ReminderName: "Test", TriggeredCount: 10, CompletedCount: 8},
				},
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/analytics/distribution?fromSec=1000&toSec=2000", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var dist AnalyticsBreakTypeDistribution
	if err := json.Unmarshal(rec.Body.Bytes(), &dist); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if dist.TotalTriggered != 10 || len(dist.Items) != 1 {
		t.Fatalf("unexpected distribution %+v", dist)
	}
}

// ---- Notification tests ----

func TestHandleNotificationCapability(t *testing.T) {
	svc := Services{
		NotificationCapabilityProvider: &fakeCapabilityProvider{
			cap: ports.NotificationCapability{
				PermissionState: ports.NotificationPermissionAuthorized,
				CanRequest:      false,
				CanOpenSettings: true,
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/notification/capability", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var cap NotificationCapability
	if err := json.Unmarshal(rec.Body.Bytes(), &cap); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if cap.PermissionState != "authorized" || cap.CanOpenSettings != true {
		t.Fatalf("unexpected capability %+v", cap)
	}
}

func TestHandleRequestNotificationPermission(t *testing.T) {
	svc := Services{
		NotificationCapabilityProvider: &fakeCapabilityProvider{
			cap: ports.NotificationCapability{
				PermissionState: ports.NotificationPermissionAuthorized,
				CanRequest:      false,
			},
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/notification/request", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var cap NotificationCapability
	if err := json.Unmarshal(rec.Body.Bytes(), &cap); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if cap.PermissionState != "authorized" {
		t.Fatalf("unexpected capability %+v", cap)
	}
}

func TestHandleRequestNotificationPermission_Error(t *testing.T) {
	svc := Services{
		NotificationCapabilityProvider: &fakeCapabilityProvider{
			err: errors.New("permission denied"),
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/notification/request", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleOpenNotificationSettings(t *testing.T) {
	svc := Services{
		NotificationCapabilityProvider: &fakeCapabilityProvider{},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/notification/open-settings", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("unexpected response %+v", resp)
	}
}

func TestHandleOpenNotificationSettings_Error(t *testing.T) {
	svc := Services{
		NotificationCapabilityProvider: &fakeCapabilityProvider{
			err: errors.New("cannot open"),
		},
	}
	server := newTestServerWithServices(t, &fakeEngine{}, svc)

	req := httptest.NewRequest(http.MethodPost, "/api/notification/open-settings", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---- Other handler tests ----

func TestHandleQuit(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})

	req := httptest.NewRequest(http.MethodPost, "/api/quit", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("unexpected response %+v", resp)
	}
}

func TestHandleRuntimeState(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{
		runtimeState: state.RuntimeState{
			GlobalEnabled: true,
			CurrentSession: &state.BreakSessionView{
				Status:       "resting",
				StartedAt:    time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
				EndsAt:       time.Date(2026, 6, 30, 10, 5, 0, 0, time.UTC),
				RemainingSec: 120,
			},
		},
	}, Services{})

	req := httptest.NewRequest(http.MethodGet, "/api/runtime", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() err=%v", err)
	}
	if resp.Status != "resting" || resp.RemainingSec != 120 {
		t.Fatalf("unexpected response %+v", resp)
	}
}

// ---- Method not allowed tests ----

func TestHandleMethodNotAllowed_AllEndpoints(t *testing.T) {
	server := newTestServerWithServices(t, &fakeEngine{}, Services{})
	handler := server.routes()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"/api/status with wrong method", http.MethodPost, "/api/status", ""},
		{"/api/pause with wrong method", http.MethodGet, "/api/pause", ""},
		{"/api/resume with wrong method", http.MethodGet, "/api/resume", ""},
		{"/api/trigger-break with wrong method", http.MethodGet, "/api/trigger-break", ""},
		{"/api/skip-break with wrong method", http.MethodGet, "/api/skip-break", ""},
		{"/api/runtime with wrong method", http.MethodPost, "/api/runtime", ""},
		{"/api/quit with wrong method", http.MethodGet, "/api/quit", ""},
		{"/api/reminders with wrong method", http.MethodPost, "/api/reminders", ""},
		{"/api/reminders/create with wrong method", http.MethodGet, "/api/reminders/create", ""},
		{"/api/reminders/update/1 with wrong method", http.MethodGet, "/api/reminders/update/1", ""},
		{"/api/reminders/delete/1 with wrong method", http.MethodGet, "/api/reminders/delete/1", ""},
		{"/api/reminders/pause/1 with wrong method", http.MethodGet, "/api/reminders/pause/1", ""},
		{"/api/reminders/resume/1 with wrong method", http.MethodGet, "/api/reminders/resume/1", ""},
		{"/api/settings with wrong method", http.MethodPost, "/api/settings", ""},
		{"/api/settings/update with wrong method", http.MethodGet, "/api/settings/update", ""},
		{"/api/settings/launch-at-login with wrong method", http.MethodPost, "/api/settings/launch-at-login", ""},
		{"/api/settings/launch-at-login/set with wrong method", http.MethodGet, "/api/settings/launch-at-login/set", ""},
		{"/api/analytics/weekly with wrong method", http.MethodPost, "/api/analytics/weekly", ""},
		{"/api/analytics/summary with wrong method", http.MethodPost, "/api/analytics/summary", ""},
		{"/api/analytics/trend with wrong method", http.MethodPost, "/api/analytics/trend", ""},
		{"/api/analytics/distribution with wrong method", http.MethodPost, "/api/analytics/distribution", ""},
		{"/api/notification/capability with wrong method", http.MethodPost, "/api/notification/capability", ""},
		{"/api/notification/request with wrong method", http.MethodGet, "/api/notification/request", ""},
		{"/api/notification/open-settings with wrong method", http.MethodGet, "/api/notification/open-settings", ""},
		{"/screenshot with wrong method", http.MethodPost, "/screenshot", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = bytes.NewBufferString(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405 for %s %s, got %d body=%s", tt.method, tt.path, rec.Code, rec.Body.String())
			}
		})
	}
}
