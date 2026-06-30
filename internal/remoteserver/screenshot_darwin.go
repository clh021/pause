//go:build darwin

package remoteserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

var runDarwinScreenshotCommand = exec.CommandContext

type darwinScreenshotCapturer struct{}

// NewScreenshotCapturer returns the macOS screenshot implementation.
func NewScreenshotCapturer() ScreenshotCapturer {
	return darwinScreenshotCapturer{}
}

func (darwinScreenshotCapturer) Capture(ctx context.Context) ([]byte, error) {
	tmp, err := os.CreateTemp("", "pause-screenshot-*.png")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	defer os.Remove(tmpPath)

	cmd := runDarwinScreenshotCommand(ctx, "screencapture", "-x", tmpPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("darwin screenshot command failed: %w: %s", err, string(output))
	}
	return os.ReadFile(tmpPath)
}
