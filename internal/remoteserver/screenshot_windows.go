//go:build windows

package remoteserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"syscall"
	"unsafe"
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
	srccopy           = 0x00CC0020
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
	procDeleteDC               = gdi32CaptureDLL.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32CaptureDLL.NewProc("CreateCompatibleBitmap")
	procDeleteObjectCapture    = gdi32CaptureDLL.NewProc("DeleteObject")
	procSelectObjectCapture    = gdi32CaptureDLL.NewProc("SelectObject")
	procBitBlt                 = gdi32CaptureDLL.NewProc("BitBlt")
	procGetDIBits              = gdi32CaptureDLL.NewProc("GetDIBits")
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

func (windowsScreenshotCapturer) Capture(context.Context) ([]byte, error) {
	desktop, _, _ := procGetDesktopWindow.Call()
	screenDC, _, err := procGetDC.Call(desktop)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC failed: %w", err)
	}
	defer procReleaseDC.Call(desktop, screenDC)

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

	ret, _, err := procBitBlt.Call(
		memDC,
		0,
		0,
		uintptr(width),
		uintptr(height),
		screenDC,
		uintptr(x),
		uintptr(y),
		srccopy,
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
