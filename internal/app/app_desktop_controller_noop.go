package app

import "context"

type noopDesktopController struct{}

func newNoopDesktopController() desktopController {
	return noopDesktopController{}
}

func (noopDesktopController) OnStartup(context.Context, *App) {}

func (noopDesktopController) PrepareForScreenshot(context.Context) (func(), error) {
	return func() {}, nil
}
