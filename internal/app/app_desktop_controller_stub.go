//go:build !wails

package app

func newDesktopController() desktopController {
	return newNoopDesktopController()
}
