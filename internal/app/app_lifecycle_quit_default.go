//go:build !wails

package app

import "pause/internal/logx"

func (a *App) Quit() {
	if a == nil {
		return
	}
	logx.Infof("app.quit ignored reason=headless_build")
	a.quitRequested.Store(true)
}
