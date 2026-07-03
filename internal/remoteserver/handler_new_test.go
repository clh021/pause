package remoteserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pause/internal/backend/bootstrap"
	settingsdomain "pause/internal/backend/domain/settings"
	"pause/internal/backend/runtime/state"
)

// ---- Force Break / Force Unlock tests ----

func TestHandleForceBreak_Success(t *testing.T) {
	engine := &fakeEngine{
		startState: state.RuntimeState{
			GlobalEnabled: true,
			CurrentSession: &state.BreakSessionView{
				Status:       "resting",
				RemainingSec: 60,
			},
		},
	}
	server := newTestServer(t, engine)

	req := httptest.NewRequest(http.MethodPost, "/api/force-break", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if resp.Status != "resting" {
		t.Fatalf("expected status 'resting', got %q", resp.Status)
	}
	if resp.RemainingSec != 60 {
		t.Fatalf("expected remainingSec 60, got %d", resp.RemainingSec)
	}
}

func TestHandleForceBreak_EngineError(t *testing.T) {
	engine := &fakeEngine{startErr: fmt.Errorf("break already active")}
	server := newTestServer(t, engine)

	req := httptest.NewRequest(http.MethodPost, "/api/force-break", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleForceUnlock_Success(t *testing.T) {
	engine := &fakeEngine{
		skipState: state.RuntimeState{GlobalEnabled: true},
	}
	server := newTestServer(t, engine)

	req := httptest.NewRequest(http.MethodPost, "/api/force-unlock", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "skip is disabled") {
		t.Fatalf("force-unlock should not be blocked by overlaySkipAllowed")
	}
}

func TestHandleForceUnlock_NoActiveBreak(t *testing.T) {
	engine := &fakeEngine{skipErr: fmt.Errorf("no active break")}
	server := newTestServer(t, engine)

	req := httptest.NewRequest(http.MethodPost, "/api/force-unlock", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- Activity Recorder tests ----

// fakeRuntimeEngine is a minimal bootstrap.RuntimeEngine for activity tests.
type fakeRuntimeEngine struct {
	bootstrap.RuntimeEngine
	rt state.RuntimeState
}

func (f *fakeRuntimeEngine) GetRuntimeState(time.Time) state.RuntimeState { return f.rt }
func (f *fakeRuntimeEngine) GetSettings() settingsdomain.Settings {
	return settingsdomain.DefaultSettings()
}
func (f *fakeRuntimeEngine) Start(context.Context) {}
func (f *fakeRuntimeEngine) Stop()                 {}

func TestActivityRecorder_TickAndGetActivity(t *testing.T) {
	dir := t.TempDir()
	origActDir := testActivityDir
	testActivityDir = func() string { return dir }
	defer func() { testActivityDir = origActDir }()

	engine := &fakeRuntimeEngine{
		rt: state.RuntimeState{LastTickActive: true, CurrentIdleSec: 0},
	}
	svc := &ScreenshotService{capturer: fakeScreenshotCapturer{png: []byte("test-png")}, dir: t.TempDir(), now: time.Now}
	rec, err := NewActivityRecorder(engine, svc, false)
	if err != nil {
		t.Fatalf("NewActivityRecorder err=%v", err)
	}
	defer rec.Close()

	now := time.Now().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		engine.rt = state.RuntimeState{LastTickActive: i%2 == 0, CurrentIdleSec: i * 10}
		iCopy := i
		rec.nowFn = func() time.Time { return now.Add(time.Duration(iCopy) * 10 * time.Second) }
		rec.Tick(context.Background())
	}
	rec.flushNow()

	from := now.Add(-10 * time.Second).Unix()
	to := now.Add(40 * time.Second).Unix()
	summary, err := rec.GetActivity(context.Background(), from, to)
	if err != nil {
		t.Fatalf("GetActivity err=%v", err)
	}
	if summary.TotalTicks != 3 {
		t.Fatalf("expected 3 ticks, got %d", summary.TotalTicks)
	}
}

func TestActivityRecorder_AutoScreenshotOnActivity(t *testing.T) {
	dir := t.TempDir()
	origScreenshotDir := testScreenshotDir
	testScreenshotDir = func() string { return dir }
	defer func() { testScreenshotDir = origScreenshotDir }()

	// Use current time so filename timestamps match query range
	now := time.Now()
	// Round to second for predictable comparison
	now = now.Truncate(time.Second)

	// Create a valid PNG
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 25), G: uint8(y * 25), B: 128, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	validPNG := pngBuf.Bytes()

	svc := &ScreenshotService{capturer: fakeScreenshotCapturer{png: validPNG}, dir: dir, now: time.Now}
	engine := &fakeRuntimeEngine{
		rt: state.RuntimeState{LastTickActive: false, CurrentIdleSec: 120},
	}
	rec, err := NewActivityRecorder(engine, svc, true)
	if err != nil {
		t.Fatalf("NewActivityRecorder err=%v", err)
	}
	defer rec.Close()

	rec.nowFn = func() time.Time { return now }
	engine.rt = state.RuntimeState{LastTickActive: true, CurrentIdleSec: 0}
	rec.Tick(context.Background())
	rec.flushNow()

	shots := listShotsInDir(dir, now.Add(-10).Unix(), now.Add(10).Unix())
	if len(shots) == 0 {
		t.Fatal("expected at least 1 shot, got 0")
	}
}

func TestActivityRecorder_NoScreenshotWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	svc := &ScreenshotService{capturer: fakeScreenshotCapturer{png: []byte("fake-png-data")}, dir: dir, now: time.Now}
	engine := &fakeRuntimeEngine{
		rt: state.RuntimeState{LastTickActive: false, CurrentIdleSec: 120},
	}
	rec, err := NewActivityRecorder(engine, svc, false)
	if err != nil {
		t.Fatalf("NewActivityRecorder err=%v", err)
	}
	defer rec.Close()

	fixedNow := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	rec.nowFn = func() time.Time { return fixedNow }
	engine.rt = state.RuntimeState{LastTickActive: true, CurrentIdleSec: 0}
	rec.Tick(context.Background())
	rec.flushNow()

	shots := listShotsInDir(dir, fixedNow.Add(-10).Unix(), fixedNow.Add(10).Unix())
	if len(shots) != 0 {
		t.Fatalf("expected 0 shots when disabled, got %d", len(shots))
	}
}

func TestActivityRecorder_CaptureOnActivityToggle(t *testing.T) {
	rec, err := NewActivityRecorder(&fakeRuntimeEngine{}, nil, false)
	if err != nil {
		t.Fatalf("NewActivityRecorder err=%v", err)
	}
	defer rec.Close()

	if rec.CaptureOnActivity() {
		t.Fatal("expected disabled initially")
	}
	rec.SetCaptureOnActivity(true)
	if !rec.CaptureOnActivity() {
		t.Fatal("expected enabled after toggle")
	}
}

// ---- HandleGetActivity tests ----

func TestHandleGetActivity_WithoutRecorder(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	req := httptest.NewRequest(http.MethodGet, "/api/activity", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var summary ActivitySummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if summary.TotalTicks != 0 {
		t.Fatalf("expected 0 ticks, got %d", summary.TotalTicks)
	}
}

func TestHandleGetActivity_WithRecorder(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	var err error
	server.activity, err = NewActivityRecorder(&fakeRuntimeEngine{}, nil, false)
	if err != nil {
		t.Fatalf("NewActivityRecorder err=%v", err)
	}
	defer server.activity.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/activity?from=0&to=9999999999", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ---- HandleServeShot tests ----

func TestHandleServeShot_Success(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	shotDir := t.TempDir()
	shotPath := filepath.Join(shotDir, "shot-2026-07-01_100000.jpg")
	if err := os.WriteFile(shotPath, []byte("fake-jpeg"), 0o644); err != nil {
		t.Fatalf("WriteFile err=%v", err)
	}

	// Override screenshot dir via package var
	origScreenshotDir := testScreenshotDir
	testScreenshotDir = func() string { return shotDir }
	defer func() { testScreenshotDir = origScreenshotDir }()

	req := httptest.NewRequest(http.MethodGet, "/shots/shot-2026-07-01_100000.jpg", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "fake-jpeg" {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestHandleServeShot_InvalidName(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	shotDir := t.TempDir()
	origScreenshotDir := testScreenshotDir
	testScreenshotDir = func() string { return shotDir }
	defer func() { testScreenshotDir = origScreenshotDir }()

	tests := []struct {
		name string
		path string
		want int // expected status code range
	}{
		{"no shot prefix", "/shots/random.jpg", 400},
		{"path traversal", "/shots/../../etc/passwd", 307}, // Go 1.22+ mux redirects cleaned paths
		{"wrong extension", "/shots/shot-test.png", 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			server.routes().ServeHTTP(rec, req)
			// Should get 4xx for invalid names, or 307 for mux redirect
			if rec.Code != tt.want && rec.Code/100 != 4 {
				t.Fatalf("expected %d or 4xx for %s, got %d body=%s", tt.want, tt.path, rec.Code, rec.Body.String())
			}
		})
	}
}

// ---- HandleAutoScreenshotSetting tests ----

func TestHandleAutoScreenshotSetting_NoRecorder(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	req := httptest.NewRequest(http.MethodGet, "/api/settings/auto-screenshot", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleAutoScreenshotSetting_GET(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	recorder, _ := NewActivityRecorder(&fakeRuntimeEngine{}, nil, true)
	server.activity = recorder
	defer recorder.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/settings/auto-screenshot", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if !resp["enabled"] {
		t.Fatalf("expected enabled=true, got %+v", resp)
	}
}

func TestHandleAutoScreenshotSetting_POST(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	recorder, _ := NewActivityRecorder(&fakeRuntimeEngine{}, nil, true)
	server.activity = recorder
	defer recorder.Close()

	body := `{"enabled": false}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings/auto-screenshot", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if resp["enabled"] {
		t.Fatalf("expected enabled=false after POST, got %+v", resp)
	}
	if recorder.CaptureOnActivity() {
		t.Fatal("recorder should have capture disabled")
	}
}

// ---- JPEG compression tests ----

func TestGoJPEGCompress(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 25), G: uint8(y * 25), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "test-output.jpg")
	if err := goJPEGCompress(buf.Bytes(), outPath, 50); err != nil {
		t.Fatalf("goJPEGCompress err=%v", err)
	}
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Fatal("output file was not created")
	}
	info, _ := os.Stat(outPath)
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
}

func TestCompressPNGToJPEGFile_Fallback(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 5, 5))
	for x := 0; x < 5; x++ {
		for y := 0; y < 5; y++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "fallback-test.jpg")
	if err := compressPNGToJPEGFile(buf.Bytes(), outPath, 55); err != nil {
		t.Fatalf("compressPNGToJPEGFile err=%v", err)
	}
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Fatal("output file was not created")
	}
}

// ---- Screenshot list test ----

func TestListShotsInDir(t *testing.T) {
	dir := t.TempDir()
	// Use names that include timestamps
	files := []string{
		"shot-2026-07-01_100000.jpg",
		"shot-2026-07-01_110000.jpg",
		"shot-2026-07-01_120000.jpg",
		"not-a-shot.png",
	}
	for _, f := range files {
		os.WriteFile(filepath.Join(dir, f), []byte("data"), 0o644)
	}

	// Parse timestamps from the filenames for range
	from := parseShotTimestamp("shot-2026-07-01_100000.jpg")
	to := parseShotTimestamp("shot-2026-07-01_120000.jpg")
	if from == 0 || to == 0 {
		t.Fatal("failed to parse reference timestamps")
	}

	shots := listShotsInDir(dir, from, to)

	if len(shots) != 3 {
		t.Fatalf("expected 3 shots, got %d", len(shots))
	}
	if shots[0].Name != "shot-2026-07-01_100000.jpg" {
		t.Fatalf("expected first shot to be 100000, got %s", shots[0].Name)
	}
}

// ---- Route registration ----

func TestNewRoutesAreRegistered(t *testing.T) {
	server := newTestServer(t, &fakeEngine{})
	tests := []string{
		"/api/force-break",
		"/api/force-unlock",
		"/api/activity",
		"/shots/test.jpg",
		"/api/settings/auto-screenshot",
		"/api/screenshots",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, path, nil)
			rec := httptest.NewRecorder()
			server.routes().ServeHTTP(rec, req)
			if rec.Code == http.StatusNotFound {
				t.Fatalf("route %s is not registered (got 404)", path)
			}
		})
	}
}
