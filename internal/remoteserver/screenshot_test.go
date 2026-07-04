package remoteserver

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeScreenshotCapturer struct {
	png []byte
	err error
}

func (f fakeScreenshotCapturer) Capture(context.Context) ([]byte, error) {
	return f.png, f.err
}

func TestStoreScreenshotBytesWritesHistory(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 30, 11, 22, 33, 0, time.UTC)
	result, err := storeScreenshotBytes(dir, []byte("png-data"), now)
	if err != nil {
		t.Fatalf("storeScreenshotBytes() err=%v", err)
	}

	wantHistory := filepath.Join(dir, "Pause_Screenshot_2026-06-30_112233.png")
	if result.HistoryPath != wantHistory {
		t.Fatalf("expected history path %q, got %q", wantHistory, result.HistoryPath)
	}

	historyBytes, err := os.ReadFile(result.HistoryPath)
	if err != nil {
		t.Fatalf("ReadFile(history) err=%v", err)
	}
	if !bytes.Equal(historyBytes, []byte("png-data")) {
		t.Fatalf("unexpected history bytes %q", string(historyBytes))
	}
}

func TestScreenshotServiceCaptureStoresFiles(t *testing.T) {
	svc := &ScreenshotService{
		capturer: fakeScreenshotCapturer{png: []byte("abc")},
		dir:      t.TempDir(),
		now: func() time.Time {
			return time.Date(2026, 6, 30, 1, 2, 3, 0, time.UTC)
		},
	}

	result, err := svc.Capture(context.Background())
	if err != nil {
		t.Fatalf("Capture() err=%v", err)
	}
	if !bytes.Equal(result.PNG, []byte("abc")) {
		t.Fatalf("unexpected png payload %q", string(result.PNG))
	}
	if _, err := os.Stat(result.HistoryPath); err != nil {
		t.Fatalf("expected history file, err=%v", err)
	}
}
