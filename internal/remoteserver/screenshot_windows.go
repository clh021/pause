//go:build windows

package remoteserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
	srccopy           = 0x00CC0020
	captureBlt        = 0x40000000
	biRGB             = 0
	dibRGBColors      = 0
)

var (
	user32CaptureDLL = syscall.NewLazyDLL("user32.dll")
	gdi32CaptureDLL  = syscall.NewLazyDLL("gdi32.dll")

	procGetDesktopWindow       = user32CaptureDLL.NewProc("GetDesktopWindow")
	procGetDC                  = user32CaptureDLL.NewProc("GetDC")
	procReleaseDC              = user32CaptureDLL.NewProc("ReleaseDC")
	procGetSystemMetrics       = user32CaptureDLL.NewProc("GetSystemMetrics")
	procCreateCompatibleDC     = gdi32CaptureDLL.NewProc("CreateCompatibleDC")
	procCreateDCW              = gdi32CaptureDLL.NewProc("CreateDCW")
	procDeleteDC               = gdi32CaptureDLL.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32CaptureDLL.NewProc("CreateCompatibleBitmap")
	procDeleteObjectCapture    = gdi32CaptureDLL.NewProc("DeleteObject")
	procSelectObjectCapture    = gdi32CaptureDLL.NewProc("SelectObject")
	procBitBlt                 = gdi32CaptureDLL.NewProc("BitBlt")
	procGetDIBits              = gdi32CaptureDLL.NewProc("GetDIBits")

	// lookupWindowsCaptureTool is overridable in tests to stub nircmd.exe lookup
	// without requiring the real binary on the build machine.
	lookupWindowsCaptureTool = exec.LookPath
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type windowsScreenshotCapturer struct{}

// NewScreenshotCapturer returns the Windows screenshot implementation.
func NewScreenshotCapturer() ScreenshotCapturer {
	return windowsScreenshotCapturer{}
}

func (windowsScreenshotCapturer) Capture(ctx context.Context) ([]byte, error) {
	// 1) Try external silent tools first (no screen flash)
	if data, err := captureViaNircmd(ctx); err == nil && len(data) > 0 {
		return data, nil
	}

	// 2) Fallback: GDI BitBlt (may cause brief flash on some drivers)
	return captureViaGDI()
}

// captureViaNircmd uses nircmd.exe (https://www.nirsoft.net/utils/nircmd.html)
// to take a silent screenshot. nircmd is a tiny freeware utility that does
// not cause any screen flash.
//
// Preference order:
//  1. Embedded nircmd.exe (bundled in production builds via //go:embed)
//  2. nircmd.exe found in PATH (for development environments)
//  3. Fall through to caller (which falls back to GDI BitBlt)
func captureViaNircmd(ctx context.Context) ([]byte, error) {
	// Try embedded nircmd.exe first (nircmdembed build tag).
	// If the embedded binary fails (e.g. architecture mismatch), fall through
	// to the PATH-based nircmd before giving up.
	if exePath, cleanup, err := getEmbeddedNircmdPath(); err == nil {
		defer cleanup()
		if data, err := runNircmd(ctx, exePath); err == nil {
			return data, nil
		}
	}

	// Fallback: look up nircmd.exe in PATH and use the resolved path.
	resolvedPath, err := lookupWindowsCaptureTool("nircmd.exe")
	if err != nil {
		return nil, err
	}
	return runNircmd(ctx, resolvedPath)
}

// runNircmd executes nircmd.exe with a savescreenshot command and returns PNG data.
// It writes a unique temp file to avoid races between concurrent captures.
//
// We use CreateTemp to generate a unique random path, then close the handle
// because nircmd.exe needs to create its own handle on the path. There is a
// tiny window between close and nircmd opening the file, but the random path
// in a private temp directory makes exploitation infeasible in practice.
func runNircmd(ctx context.Context, exePath string) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "pause-ss-nircmd-*.png")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	cmd := exec.CommandContext(ctx, exePath, "savescreenshot", tmpPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("nircmd failed: %w: %s", err, string(output))
	}
	defer os.Remove(tmpPath)
	return os.ReadFile(tmpPath)
}

func captureViaGDI() ([]byte, error) {
	// Try CreateDC("DISPLAY") first — reads directly from the display driver,
	// bypassing DWM composition. This reduces the chance of a flash on most systems.
	screenDC, _, err := procCreateDCW.Call(
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("DISPLAY"))), 0, 0, 0)
	if screenDC == 0 {
		// Fallback: classic GetDC(GetDesktopWindow())
		desktop, _, _ := procGetDesktopWindow.Call()
		screenDC, _, err = procGetDC.Call(desktop)
		if screenDC == 0 {
			return nil, fmt.Errorf("GetDC failed: %w", err)
		}
		defer procReleaseDC.Call(desktop, screenDC)
	} else {
		defer procDeleteDC.Call(screenDC)
	}

	memDC, _, err := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed: %w", err)
	}
	defer procDeleteDC.Call(memDC)

	x := getSystemMetric(smXVirtualScreen)
	y := getSystemMetric(smYVirtualScreen)
	width := getSystemMetric(smCXVirtualScreen)
	height := getSystemMetric(smCYVirtualScreen)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid virtual screen size %dx%d", width, height)
	}

	bitmap, _, err := procCreateCompatibleBitmap.Call(screenDC, uintptr(width), uintptr(height))
	if bitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed: %w", err)
	}
	defer procDeleteObjectCapture.Call(bitmap)

	oldObj, _, _ := procSelectObjectCapture.Call(memDC, bitmap)
	defer procSelectObjectCapture.Call(memDC, oldObj)

	// SRCCOPY | CAPTUREBLT — CAPTUREBLT tells DWM this is a screenshot
	// so it can handle the frame correctly without visible artifacts.
	ret, _, err := procBitBlt.Call(
		memDC,
		0,
		0,
		uintptr(width),
		uintptr(height),
		screenDC,
		uintptr(x),
		uintptr(y),
		srccopy|captureBlt,
	)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt failed: %w", err)
	}

	info := bitmapInfo{
		Header: bitmapInfoHeader{
			Size:        uint32(binary.Size(bitmapInfoHeader{})),
			Width:       int32(width),
			Height:      -int32(height),
			Planes:      1,
			BitCount:    32,
			Compression: biRGB,
		},
	}
	raw := make([]byte, width*height*4)
	ret, _, err = procGetDIBits.Call(
		memDC,
		bitmap,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&raw[0])),
		uintptr(unsafe.Pointer(&info)),
		dibRGBColors,
	)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed: %w", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(raw); i += 4 {
		img.Pix[i] = raw[i+2]
		img.Pix[i+1] = raw[i+1]
		img.Pix[i+2] = raw[i]
		img.Pix[i+3] = 0xFF
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func getSystemMetric(idx int) int {
	value, _, _ := procGetSystemMetrics.Call(uintptr(idx))
	return int(int32(value))
}
