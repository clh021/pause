package app

import (
	"fmt"
	"strings"

	"pause/internal/backend/domain/settings"
	"pause/internal/backend/runtime/state"
)

func overlaySkipMode(settings settings.Settings) skipMode {
	if settings.Enforcement.OverlaySkipAllowed {
		return skipModeNormal
	}
	return skipModeEmergency
}

func overlayReasons(state state.RuntimeState) string {
	if state.CurrentSession == nil || len(state.CurrentSession.Reasons) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(state.CurrentSession.Reasons))
	for _, reason := range state.CurrentSession.Reasons {
		parts = append(parts, fmt.Sprintf("%d", reason))
	}
	return strings.Join(parts, "+")
}

func overlayRemainingSec(state state.RuntimeState) int {
	if state.CurrentSession == nil {
		return 0
	}
	return state.CurrentSession.RemainingSec
}
