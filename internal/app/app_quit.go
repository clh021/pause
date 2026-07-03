package app

func (a *App) setQuitFunc(fn func()) {
	if a == nil {
		return
	}
	a.quitMu.Lock()
	a.quitFunc = fn
	a.quitMu.Unlock()
}

func (a *App) runQuitFunc() bool {
	if a == nil {
		return false
	}
	a.quitMu.Lock()
	fn := a.quitFunc
	a.quitMu.Unlock()
	if fn == nil {
		return false
	}
	fn()
	return true
}
