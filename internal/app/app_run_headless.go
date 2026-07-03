package app

import (
	"context"
	"os/signal"
	"syscall"

	"pause/internal/logx"
)

func RunHeadless(configPath string) error {
	desktopApp, err := NewApp(configPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	desktopApp.Startup(ctx)
	info := desktopApp.GetRemoteServerInfo()
	if info.Running {
		logx.Infof("app.headless_remote url=%s token_required=%t", info.LocalBaseURL, info.TokenRequired)
	}
	logx.Infof("app.headless_running")
	<-ctx.Done()
	logx.Infof("app.headless_stopped")
	return nil
}
