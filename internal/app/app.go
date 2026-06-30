package app

import (
	"context"
	"time"

	"pause/internal/backend/bootstrap"
	"pause/internal/logx"
	"pause/internal/meta"
	"pause/internal/paths"
	"pause/internal/remoteserver"
)

func NewApp(configPath string) (*App, error) {
	if configPath == "" {
		resolved, err := defaultConfigPath()
		if err != nil {
			return nil, err
		}
		configPath = resolved
	}

	runtime, err := bootstrap.NewRuntime(configPath, meta.EffectiveAppBundleID())
	if err != nil {
		return nil, err
	}
	if runtime.Settings.WasCreated() {
		language := resolveEffectiveLanguage(runtime.Settings.Get().UI.Language)
		if err := ensureBuiltInRemindersForFirstInstall(context.Background(), runtime.ReminderService, language); err != nil {
			if closeErr := runtime.Close(); closeErr != nil {
				logx.Warnf("app.init cleanup_close_err stage=seed_builtin_reminders err=%v", closeErr)
			}
			return nil, err
		}
	}

	logx.Infof(
		"app.init bundle_id=%s config_path=%s history_path=%s config_created=%t",
		meta.EffectiveAppBundleID(),
		configPath,
		runtime.HistoryPath,
		runtime.Settings.WasCreated(),
	)

	return &App{
		engine:                 runtime.Engine,
		runtime:                runtime,
		reminders:              runtime.ReminderService,
		analytics:              runtime.AnalyticsService,
		settingsSvc:            runtime.SettingsService,
		notifier:               runtime.Notifier,
		notificationCapability: runtime.NotificationCapabilityProvider,
		desktop:                newDesktopController(),
	}, nil
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	if a.settingsSvc != nil {
		if err := a.settingsSvc.SyncPlatformSettings(appContextOrBackground(ctx)); err != nil {
			logx.Warnf("app.startup sync_platform_settings_err=%v", err)
		}
	} else {
		logx.Warnf("app.startup sync_platform_settings_skipped reason=settings_service_unavailable")
	}
	a.engine.Start(ctx)
	if a.desktop != nil {
		a.desktop.OnStartup(ctx, a)
	}
	if err := a.startRemoteServer(ctx); err != nil {
		logx.Warnf("app.startup remote_server_err=%v", err)
	}
	logx.Infof("app.startup completed")
}

func defaultConfigPath() (string, error) {
	return paths.ConfigFile("settings.json")
}

func appContextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func (a *App) Shutdown(_ context.Context) {
	if a == nil || a.runtime == nil {
		return
	}
	logx.Infof("app.shutdown started")
	if a.remoteServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := a.remoteServer.Shutdown(shutdownCtx); err != nil {
			logx.Warnf("app.shutdown remote_server_close_err=%v", err)
		}
		cancel()
	}
	if err := a.runtime.Close(); err != nil {
		logx.Warnf("app.shutdown runtime_close_err=%v", err)
		return
	}
	logx.Infof("app.shutdown completed")
}

func (a *App) startRemoteServer(ctx context.Context) error {
	cfg, err := remoteserver.LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return nil
	}
	server, err := remoteserver.NewServer(cfg, a.engine, a.desktop.PrepareForScreenshot)
	if err != nil {
		return err
	}
	if err := server.Start(ctx); err != nil {
		return err
	}
	a.remoteServer = server
	return nil
}
