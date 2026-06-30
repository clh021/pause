package remoteserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var resolveScreenshotHomeDir = os.UserHomeDir

const latestScreenshotName = "Pause_Screenshot_Latest.png"

// ScreenshotCapturer captures the current desktop into a PNG payload.
type ScreenshotCapturer interface {
	Capture(ctx context.Context) ([]byte, error)
}

// ScreenshotResult describes the stored screenshot artifacts for one capture.
type ScreenshotResult struct {
	HistoryPath string
	LatestPath  string
	PNG         []byte
}

// ScreenshotService coordinates capture and on-disk persistence.
type ScreenshotService struct {
	capturer ScreenshotCapturer
	dir      string
	now      func() time.Time
}

// NewScreenshotService builds a screenshot service with the platform capturer.
func NewScreenshotService(capturer ScreenshotCapturer) (*ScreenshotService, error) {
	if capturer == nil {
		return nil, errors.New("screenshot capturer is required")
	}
	dir, err := screenshotsDir()
	if err != nil {
		return nil, err
	}
	return &ScreenshotService{
		capturer: capturer,
		dir:      dir,
		now:      time.Now,
	}, nil
}

// Capture stores a timestamped screenshot and refreshes the latest snapshot.
func (s *ScreenshotService) Capture(ctx context.Context) (ScreenshotResult, error) {
	if s == nil || s.capturer == nil {
		return ScreenshotResult{}, errors.New("screenshot service unavailable")
	}
	png, err := s.capturer.Capture(ctx)
	if err != nil {
		return ScreenshotResult{}, err
	}
	return storeScreenshotBytes(s.dir, png, s.now())
}

func storeScreenshotBytes(dir string, png []byte, now time.Time) (ScreenshotResult, error) {
	if len(png) == 0 {
		return ScreenshotResult{}, errors.New("empty screenshot image")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ScreenshotResult{}, err
	}

	historyPath := filepath.Join(dir, screenshotFileName(now))
	if err := os.WriteFile(historyPath, png, 0o644); err != nil {
		return ScreenshotResult{}, err
	}

	latestPath := filepath.Join(dir, latestScreenshotName)
	if err := os.WriteFile(latestPath, png, 0o644); err != nil {
		return ScreenshotResult{}, err
	}

	return ScreenshotResult{
		HistoryPath: historyPath,
		LatestPath:  latestPath,
		PNG:         png,
	}, nil
}

func screenshotsDir() (string, error) {
	home, err := resolveScreenshotHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(home) == "" {
		return "", errors.New("unable to resolve home directory")
	}
	return filepath.Join(home, ".pause", "screenshots"), nil
}

func screenshotFileName(now time.Time) string {
	return fmt.Sprintf("Pause_Screenshot_%s.png", now.Format("2006-01-02_150405"))
}
