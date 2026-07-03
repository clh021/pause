package remoteserver

import (
	"context"
	"net/http"
	"os"
	"time"

	settingsdomain "pause/internal/backend/domain/settings"
	runtimestate "pause/internal/backend/runtime/state"
	"pause/internal/platform"
)

type ReminderRuntime struct {
	ID           int64  `json:"id"`
	Name         string `json:"name,omitempty"`
	ReminderType string `json:"reminderType,omitempty"`
	Enabled      bool   `json:"enabled"`
	Paused       bool   `json:"paused"`
	NextInSec    int    `json:"nextInSec"`
	IntervalSec  int    `json:"intervalSec"`
	BreakSec     int    `json:"breakSec"`
}

type BreakSessionView struct {
	Status       string    `json:"status"`
	Reasons      []int64   `json:"reasons"`
	StartedAt    time.Time `json:"startedAt"`
	EndsAt       time.Time `json:"endsAt"`
	RemainingSec int       `json:"remainingSec"`
	CanSkip      bool      `json:"canSkip"`
	CanPostpone  bool      `json:"canPostpone"`
}

type RuntimeState struct {
	Now                time.Time         `json:"now"`
	CurrentSession     *BreakSessionView `json:"currentSession,omitempty"`
	Reminders          []ReminderRuntime `json:"reminders"`
	NextBreakReason    []int64           `json:"nextBreakReason"`
	GlobalEnabled      bool              `json:"globalEnabled"`
	TimerMode          string            `json:"timerMode"`
	IdleThresholdSec   int               `json:"idleThresholdSec"`
	LastTickActive     bool              `json:"lastTickActive"`
	CurrentIdleSec     int               `json:"currentIdleSec"`
	ShowTrayCountdown  bool              `json:"showTrayCountdown"`
	OverlaySkipAllowed bool              `json:"overlaySkipAllowed"`
	OverlayNative      bool              `json:"overlayNative"`
	EffectiveLanguage  string            `json:"effectiveLanguage,omitempty"`
	EffectiveTheme     string            `json:"effectiveTheme,omitempty"`
}

func (s *Server) currentSettings(ctx context.Context) settingsdomain.Settings {
	if s != nil && s.services.SettingsService != nil {
		return s.services.SettingsService.Get(ctx).Normalize()
	}
	return settingsdomain.DefaultSettings().Normalize()
}

func (s *Server) writeRuntimeState(w http.ResponseWriter, status int, runtimeState runtimestate.RuntimeState, ctx context.Context) {
	writeJSON(w, status, runtimeStateToDTO(runtimeState, s.currentSettings(ctx)))
}

func runtimeStateToDTO(runtimeState runtimestate.RuntimeState, settings settingsdomain.Settings) RuntimeState {
	settings = settings.Normalize()

	dto := RuntimeState{
		Now:                runtimeState.Now,
		Reminders:          make([]ReminderRuntime, 0, len(runtimeState.Reminders)),
		NextBreakReason:    append([]int64(nil), runtimeState.NextBreakReason...),
		GlobalEnabled:      runtimeState.GlobalEnabled,
		TimerMode:          runtimeState.TimerMode,
		IdleThresholdSec:   runtimeState.IdleThresholdSec,
		LastTickActive:     runtimeState.LastTickActive,
		CurrentIdleSec:     runtimeState.CurrentIdleSec,
		ShowTrayCountdown:  runtimeState.ShowTrayCountdown,
		OverlaySkipAllowed: runtimeState.OverlaySkipAllowed,
		OverlayNative:      runtimeState.OverlayNative,
		EffectiveLanguage:  runtimeState.EffectiveLanguage,
		EffectiveTheme:     runtimeState.EffectiveTheme,
	}

	if dto.EffectiveLanguage == "" {
		dto.EffectiveLanguage = resolveRemoteEffectiveLanguage(settings.UI.Language)
	}
	if dto.EffectiveTheme == "" {
		dto.EffectiveTheme = resolveRemoteEffectiveTheme(settings.UI.Theme)
	}

	if runtimeState.CurrentSession != nil {
		dto.CurrentSession = &BreakSessionView{
			Status:       runtimeState.CurrentSession.Status,
			Reasons:      append([]int64(nil), runtimeState.CurrentSession.Reasons...),
			StartedAt:    runtimeState.CurrentSession.StartedAt,
			EndsAt:       runtimeState.CurrentSession.EndsAt,
			RemainingSec: runtimeState.CurrentSession.RemainingSec,
			CanSkip:      runtimeState.CurrentSession.CanSkip,
			CanPostpone:  runtimeState.CurrentSession.CanPostpone,
		}
	}

	for _, reminder := range runtimeState.Reminders {
		dto.Reminders = append(dto.Reminders, ReminderRuntime{
			ID:           reminder.ID,
			Name:         reminder.Name,
			ReminderType: reminder.ReminderType,
			Enabled:      reminder.Enabled,
			Paused:       reminder.Paused,
			NextInSec:    reminder.NextInSec,
			IntervalSec:  reminder.IntervalSec,
			BreakSec:     reminder.BreakSec,
		})
	}

	return dto
}

func resolveRemoteEffectiveLanguage(setting string) string {
	preferredLocales := []string{platform.DetectPreferredLanguage()}
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		preferredLocales = append(preferredLocales, os.Getenv(key))
	}
	return settingsdomain.ResolveEffectiveUILanguage(setting, preferredLocales...)
}

func resolveRemoteEffectiveTheme(setting string) string {
	normalized := settingsdomain.NormalizeUITheme(setting)
	if normalized == settingsdomain.UIThemeLight || normalized == settingsdomain.UIThemeDark {
		return normalized
	}
	return settingsdomain.UIThemeDark
}
