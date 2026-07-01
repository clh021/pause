//go:build linux

package remoteserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var (
	lookupScreenshotCommand = exec.LookPath
	runScreenshotCommand    = exec.CommandContext
)

type linuxScreenshotCapturer struct{}

// NewScreenshotCapturer returns the Linux screenshot implementation.
func NewScreenshotCapturer() ScreenshotCapturer {
	return linuxScreenshotCapturer{}
}

func (linuxScreenshotCapturer) Capture(ctx context.Context) ([]byte, error) {
	name, args, err := linuxScreenshotCommandSpec("")
	if err != nil {
		return nil, err
	}
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

	_, args, _ = linuxScreenshotCommandSpec(tmpPath)
	cmd := runScreenshotCommand(ctx, name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("linux screenshot command failed: %w: %s", err, string(output))
	}
	return os.ReadFile(tmpPath)
}

func linuxScreenshotCommandSpec(outputPath string) (string, []string, error) {
	// ImageMagick import (X11, silent with -window root)
	if _, err := lookupScreenshotCommand("import"); err == nil {
		return "import", []string{"-window", "root", outputPath}, nil
	}
	// KDE spectacle (Wayland/X11, silent with -b -n)
	if _, err := lookupScreenshotCommand("spectacle"); err == nil {
		return "spectacle", []string{"-b", "-n", "-o", outputPath}, nil
	}
	// grim for wlroots-based Wayland compositors (Sway, Hyprland, etc.)
	if _, err := lookupScreenshotCommand("grim"); err == nil {
		return "grim", []string{outputPath}, nil
	}
	// Fallback: gnome-screenshot (may flash on some setups)
	if _, err := lookupScreenshotCommand("gnome-screenshot"); err == nil {
		return "gnome-screenshot", []string{"-f", outputPath}, nil
	}
	return "", nil, errors.New("no supported linux screenshot tool found: install imagemagick, spectacle, grim, or gnome-screenshot")
}
