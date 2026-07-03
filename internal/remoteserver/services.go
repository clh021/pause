package remoteserver

import (
	"context"

	"pause/internal/backend/bootstrap"
	analyticsdomain "pause/internal/backend/domain/analytics"
	reminderdomain "pause/internal/backend/domain/reminder"
	settingsdomain "pause/internal/backend/domain/settings"
	"pause/internal/backend/ports"
)

// Services bundles all app services for the remote HTTP server.
type Services struct {
	Engine                         bootstrap.RuntimeEngine
	ReminderService                ReminderService
	AnalyticsService               AnalyticsService
	SettingsService                SettingsService
	NotificationCapabilityProvider ports.NotificationCapabilityProvider
	Quit                           func()
}

// --- Reminder DTOs ---

type ReminderConfig struct {
	ID           int64  `json:"id"`
	Name         string `json:"name,omitempty"`
	Enabled      bool   `json:"enabled"`
	IntervalSec  int    `json:"intervalSec"`
	BreakSec     int    `json:"breakSec"`
	ReminderType string `json:"reminderType,omitempty"`
}

type ReminderCreateInput struct {
	Name         string  `json:"name"`
	IntervalSec  int     `json:"intervalSec"`
	BreakSec     int     `json:"breakSec"`
	Enabled      *bool   `json:"enabled,omitempty"`
	ReminderType *string `json:"reminderType,omitempty"`
}

type ReminderPatch struct {
	ID           int64   `json:"id"`
	Name         *string `json:"name,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
	IntervalSec  *int    `json:"intervalSec,omitempty"`
	BreakSec     *int    `json:"breakSec,omitempty"`
	ReminderType *string `json:"reminderType,omitempty"`
}

// --- Settings DTOs ---

type Settings struct {
	Enforcement EnforcementSettings `json:"enforcement"`
	Sound       SoundSettings       `json:"sound"`
	Timer       TimerSettings       `json:"timer"`
	UI          UISettings          `json:"ui"`
}

type EnforcementSettings struct {
	OverlaySkipAllowed bool `json:"overlaySkipAllowed"`
}

type SoundSettings struct {
	Enabled bool `json:"enabled"`
}

type TimerSettings struct {
	Mode                  string `json:"mode"`
	IdlePauseThresholdSec int    `json:"idlePauseThresholdSec"`
}

type UISettings struct {
	ShowTrayCountdown bool   `json:"showTrayCountdown"`
	Language          string `json:"language"`
	Theme             string `json:"theme"`
}

type SettingsPatch struct {
	Enforcement *EnforcementSettingsPatch `json:"enforcement,omitempty"`
	Sound       *SoundSettingsPatch       `json:"sound,omitempty"`
	Timer       *TimerSettingsPatch       `json:"timer,omitempty"`
	UI          *UISettingsPatch          `json:"ui,omitempty"`
}

type EnforcementSettingsPatch struct {
	OverlaySkipAllowed *bool `json:"overlaySkipAllowed,omitempty"`
}

type SoundSettingsPatch struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type TimerSettingsPatch struct {
	Mode                  *string `json:"mode,omitempty"`
	IdlePauseThresholdSec *int    `json:"idlePauseThresholdSec,omitempty"`
}

type UISettingsPatch struct {
	ShowTrayCountdown *bool   `json:"showTrayCountdown,omitempty"`
	Language          *string `json:"language,omitempty"`
	Theme             *string `json:"theme,omitempty"`
}

// --- Analytics DTOs ---

type AnalyticsWeeklyStats struct {
	FromSec   int64                   `json:"fromSec"`
	ToSec     int64                   `json:"toSec"`
	Reminders []AnalyticsReminderStat `json:"reminders"`
	Summary   AnalyticsSummaryStats   `json:"summary"`
}

type AnalyticsReminderStat struct {
	ReminderID          int64   `json:"reminderId"`
	ReminderName        string  `json:"reminderName"`
	Enabled             bool    `json:"enabled"`
	ReminderType        string  `json:"reminderType"`
	TriggeredCount      int     `json:"triggeredCount"`
	CompletedCount      int     `json:"completedCount"`
	SkippedCount        int     `json:"skippedCount"`
	TotalActualBreakSec int     `json:"totalActualBreakSec"`
	AvgActualBreakSec   float64 `json:"avgActualBreakSec"`
}

type AnalyticsSummaryStats struct {
	TotalSessions       int     `json:"totalSessions"`
	TotalCompleted      int     `json:"totalCompleted"`
	TotalSkipped        int     `json:"totalSkipped"`
	TotalActualBreakSec int     `json:"totalActualBreakSec"`
	AvgActualBreakSec   float64 `json:"avgActualBreakSec"`
}

type AnalyticsSummary struct {
	FromSec             int64   `json:"fromSec"`
	ToSec               int64   `json:"toSec"`
	TotalSessions       int     `json:"totalSessions"`
	TotalCompleted      int     `json:"totalCompleted"`
	TotalSkipped        int     `json:"totalSkipped"`
	CompletionRate      float64 `json:"completionRate"`
	SkipRate            float64 `json:"skipRate"`
	TotalActualBreakSec int     `json:"totalActualBreakSec"`
	AvgActualBreakSec   float64 `json:"avgActualBreakSec"`
}

type AnalyticsTrend struct {
	FromSec int64                 `json:"fromSec"`
	ToSec   int64                 `json:"toSec"`
	Points  []AnalyticsTrendPoint `json:"points"`
}

type AnalyticsTrendPoint struct {
	Day                 string  `json:"day"`
	TotalSessions       int     `json:"totalSessions"`
	TotalCompleted      int     `json:"totalCompleted"`
	TotalSkipped        int     `json:"totalSkipped"`
	CompletionRate      float64 `json:"completionRate"`
	SkipRate            float64 `json:"skipRate"`
	TotalActualBreakSec int     `json:"totalActualBreakSec"`
	AvgActualBreakSec   float64 `json:"avgActualBreakSec"`
}

type AnalyticsBreakTypeDistribution struct {
	FromSec        int64                                `json:"fromSec"`
	ToSec          int64                                `json:"toSec"`
	TotalTriggered int                                  `json:"totalTriggered"`
	Items          []AnalyticsBreakTypeDistributionItem `json:"items"`
}

type AnalyticsBreakTypeDistributionItem struct {
	ReminderID      int64   `json:"reminderId"`
	ReminderName    string  `json:"reminderName"`
	TriggeredCount  int     `json:"triggeredCount"`
	CompletedCount  int     `json:"completedCount"`
	SkippedCount    int     `json:"skippedCount"`
	CompletionRate  float64 `json:"completionRate"`
	SkipRate        float64 `json:"skipRate"`
	TriggeredShare  float64 `json:"triggeredShare"`
	ReminderType    string  `json:"reminderType,omitempty"`
	ReminderEnabled bool    `json:"reminderEnabled"`
}

// --- Notification DTOs ---

type NotificationCapability struct {
	PermissionState string `json:"permissionState"`
	CanRequest      bool   `json:"canRequest"`
	CanOpenSettings bool   `json:"canOpenSettings"`
	Reason          string `json:"reason,omitempty"`
}

// --- Domain service interfaces ---

type ReminderService interface {
	List(ctx context.Context) ([]reminderdomain.Reminder, error)
	Create(ctx context.Context, input reminderdomain.CreateInput) ([]reminderdomain.Reminder, error)
	Update(ctx context.Context, patch reminderdomain.Patch) ([]reminderdomain.Reminder, error)
	Delete(ctx context.Context, reminderID int64) ([]reminderdomain.Reminder, error)
}

type AnalyticsService interface {
	GetWeeklyStats(ctx context.Context, fromSec int64, toSec int64) (analyticsdomain.WeeklyStats, error)
	GetSummary(ctx context.Context, fromSec int64, toSec int64) (analyticsdomain.Summary, error)
	GetTrendByDay(ctx context.Context, fromSec int64, toSec int64) (analyticsdomain.Trend, error)
	GetBreakTypeDistribution(ctx context.Context, fromSec int64, toSec int64) (analyticsdomain.BreakTypeDistribution, error)
}

type SettingsService interface {
	Get(ctx context.Context) settingsdomain.Settings
	Update(ctx context.Context, patch settingsdomain.SettingsPatch) (settingsdomain.Settings, error)
	GetLaunchAtLogin(ctx context.Context) (bool, error)
	SetLaunchAtLogin(ctx context.Context, enabled bool) (bool, error)
}

// NotificationCapabilityProvider re-exported from ports for convenience.
type NotificationCapabilityProvider = ports.NotificationCapabilityProvider
