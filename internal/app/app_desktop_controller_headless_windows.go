//go:build windows && wails

package app

import (
	"context"
	"sync/atomic"
	"time"

	"pause/internal/backend/domain/settings"
	runtimestate "pause/internal/backend/runtime/state"
	"pause/internal/desktop"
	"pause/internal/logx"
)

// headlessDesktopController manages the native break overlay in headless/WinWS
// mode (no Wails main window). It replicates the overlay-sync loop from
// wailsDesktopController without any Wails runtime calls (no status bar, no
// log sink, no main-window hide/show).
type headlessDesktopController struct {
	overlay                desktop.BreakOverlayController
	screenshotSuspendUntil atomic.Int64

	lastLanguage           string
	lastOverlayActive      bool
	lastOverlaySkip        bool
	lastOverlayPostpone    bool
	lastOverlayLang        string
	lastOverlayText        string
	lastOverlayTheme       string
	lastOverlaySessionStart time.Time
	overlayFailureLogged   bool
}

func newHeadlessDesktopController() desktopController {
	return &headlessDesktopController{
		overlay: desktop.NewBreakOverlayController(),
	}
}

func (c *headlessDesktopController) OnStartup(ctx context.Context, app *App) {
	settings := app.engine.GetSettings()
	c.lastLanguage = resolveEffectiveLanguage(settings.UI.Language)

	logx.Infof("headless.overlay.init language=%s", c.lastLanguage)
	c.overlay.Init(
		func() {
			skipMode := overlaySkipMode(app.engine.GetSettings())
			_, err := app.skipCurrentBreakWithMode(skipMode)
			if err != nil {
				logx.Errorf("headless.overlay.skip_err err=%v", err)
			}
		},
		func() {
			_, err := app.PostponeCurrentBreak()
			if err != nil {
				logx.Errorf("headless.overlay.postpone_err err=%v", err)
			}
		},
	)

	if c.overlay.IsNative() {
		logx.Infof("headless.overlay.started native=true")
	} else {
		logx.Warnf("headless.overlay.started native=false")
	}
	go c.runtimeLoop(ctx, app)

	logx.Infof("headless.overlay.sync_loop_started")
}

func (c *headlessDesktopController) runtimeLoop(ctx context.Context, app *App) {
	logx.Infof("headless.overlay.sync_loop_running")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer func() {
		c.overlay.Destroy()
		logx.Infof("headless.overlay.destroyed")
	}()

	settings := app.engine.GetSettings()
	state := app.engine.GetRuntimeState(time.Now())
	c.syncOverlay(state, settings)

	for {
		select {
		case <-ctx.Done():
			logx.Infof("headless.overlay.sync_loop_stopped")
			return
		case now := <-ticker.C:
			settings := app.engine.GetSettings()
			state := app.engine.GetRuntimeState(now)
			c.syncOverlay(state, settings)
		}
	}
}

func (c *headlessDesktopController) syncOverlay(state runtimestate.RuntimeState, settings settings.Settings) {
	if until := c.screenshotSuspendUntil.Load(); until > 0 && time.Now().UnixNano() < until {
		c.overlay.Hide()
		c.lastOverlayActive = false
		c.lastOverlaySkip = false
		c.lastOverlayPostpone = false
		c.lastOverlayLang = c.lastLanguage
		c.lastOverlayText = ""
		c.lastOverlayTheme = resolveEffectiveTheme(settings.UI.Theme)
		return
	}

	overlayActive := state.CurrentSession != nil && state.CurrentSession.Status == "resting"
	overlaySkipAllowed := overlayActive && state.OverlaySkipAllowed && state.CurrentSession != nil && state.CurrentSession.CanSkip
	overlayPostponeAllowed := overlayActive && state.CurrentSession != nil && state.CurrentSession.CanPostpone
	language := c.lastLanguage
	theme := resolveEffectiveTheme(settings.UI.Theme)
	overlayText := ""
	overlayMessage := overlayMessageText(language)
	if overlayActive && state.CurrentSession != nil {
		overlayText = overlayCountdownText(language, state.CurrentSession.RemainingSec)
		if !state.CurrentSession.StartedAt.Equal(c.lastOverlaySessionStart) {
			c.lastOverlaySessionStart = state.CurrentSession.StartedAt
			c.overlayFailureLogged = false
		}
	} else {
		c.lastOverlaySessionStart = time.Time{}
		c.overlayFailureLogged = false
	}

	if c.overlay.IsNative() {
		transition := overlayActive != c.lastOverlayActive

		needsUpdate := transition || overlaySkipAllowed != c.lastOverlaySkip ||
			overlayPostponeAllowed != c.lastOverlayPostpone ||
			language != c.lastOverlayLang ||
			overlayText != c.lastOverlayText ||
			theme != c.lastOverlayTheme
		if needsUpdate {
			if overlayActive {
				if transition {
					logx.Infof("headless.overlay.shown reasons=%s remaining_sec=%d skip_allowed=%t postpone_allowed=%t",
						overlayReasons(state),
						overlayRemainingSec(state),
						overlaySkipAllowed,
						overlayPostponeAllowed,
					)
				}
				// No HideMainWindowForOverlay here — there is no Wails main window
				// in headless mode. Calling Wails runtime functions would crash.
				if !c.overlay.Show(overlaySkipAllowed, overlaySkipButtonTitle(language), overlayPostponeAllowed, overlayPostponeButtonTitle(language), overlayText, overlayMessage, theme) {
					if !c.overlayFailureLogged {
						logx.Warnf(
							"overlay.show_failed native=true reasons=%s remaining_sec=%d",
							overlayReasons(state),
							overlayRemainingSec(state),
						)
						c.overlayFailureLogged = true
					}
				}
			} else {
				if transition {
					logx.Infof("headless.overlay.hidden")
				}
				c.overlay.Hide()
			}
		}
		c.lastOverlayActive = overlayActive
		c.lastOverlaySkip = overlaySkipAllowed
		c.lastOverlayPostpone = overlayPostponeAllowed
		c.lastOverlayLang = language
		c.lastOverlayText = overlayText
		c.lastOverlayTheme = theme
		return
	}

	if overlayActive && !c.overlayFailureLogged {
		logx.Warnf(
			"overlay.show_failed native=false reasons=%s remaining_sec=%d",
			overlayReasons(state),
			overlayRemainingSec(state),
		)
		c.overlayFailureLogged = true
	}
	c.lastOverlayActive = overlayActive
	c.lastOverlaySkip = overlaySkipAllowed
	c.lastOverlayPostpone = overlayPostponeAllowed
	c.lastOverlayLang = language
	c.lastOverlayText = overlayText
	c.lastOverlayTheme = theme
}

func (c *headlessDesktopController) PrepareForScreenshot(_ context.Context) (func(), error) {
	logx.Infof("headless.overlay.screenshot_prepare")
	c.screenshotSuspendUntil.Store(time.Now().Add(2 * time.Second).UnixNano())
	c.overlay.Hide()
	return func() {
		c.screenshotSuspendUntil.Store(0)
		logx.Infof("headless.overlay.screenshot_resume")
	}, nil
}
