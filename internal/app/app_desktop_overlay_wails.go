//go:build wails

package app

import (
	"context"
	"time"

	"pause/internal/backend/domain/settings"
	"pause/internal/backend/runtime/state"
	"pause/internal/desktop"
	"pause/internal/logx"
)

func (c *wailsDesktopController) syncOverlay(ctx context.Context, state state.RuntimeState, settings settings.Settings) {
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
		needsUpdate := overlayActive != c.lastOverlayActive || overlaySkipAllowed != c.lastOverlaySkip || overlayPostponeAllowed != c.lastOverlayPostpone || language != c.lastOverlayLang || overlayText != c.lastOverlayText || theme != c.lastOverlayTheme
		if needsUpdate {
			if overlayActive {
				// Keep native break overlay isolated from the main window.
				desktop.HideMainWindowForOverlay(ctx)
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

