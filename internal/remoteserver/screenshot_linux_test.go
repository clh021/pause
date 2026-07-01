//go:build linux

package remoteserver

import (
	"errors"
	"testing"
)

func TestLinuxScreenshotCommandSpecPrefersImport(t *testing.T) {
	original := lookupScreenshotCommand
	t.Cleanup(func() {
		lookupScreenshotCommand = original
	})
	lookupScreenshotCommand = func(name string) (string, error) {
		if name == "import" {
			return "/usr/bin/import", nil
		}
		return "", errors.New("missing")
	}

	name, args, err := linuxScreenshotCommandSpec("/tmp/out.png")
	if err != nil {
		t.Fatalf("linuxScreenshotCommandSpec() err=%v", err)
	}
	if name != "import" {
		t.Fatalf("expected import, got %q", name)
	}
	if len(args) != 3 || args[2] != "/tmp/out.png" {
		t.Fatalf("unexpected args %#v", args)
	}
}

func TestLinuxScreenshotCommandSpecFallsBackToGnomeScreenshot(t *testing.T) {
	original := lookupScreenshotCommand
	t.Cleanup(func() {
		lookupScreenshotCommand = original
	})
	lookupScreenshotCommand = func(name string) (string, error) {
		if name == "gnome-screenshot" {
			return "/usr/bin/gnome-screenshot", nil
		}
		return "", errors.New("missing")
	}

	name, args, err := linuxScreenshotCommandSpec("/tmp/out.png")
	if err != nil {
		t.Fatalf("linuxScreenshotCommandSpec() err=%v", err)
	}
	if name != "gnome-screenshot" {
		t.Fatalf("expected gnome-screenshot, got %q", name)
	}
	if len(args) != 2 || args[1] != "/tmp/out.png" {
		t.Fatalf("unexpected args %#v", args)
	}
}
