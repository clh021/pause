//go:build !windows || !wails

package app

func newHeadlessDesktopController() desktopController {
	return newNoopDesktopController()
}
