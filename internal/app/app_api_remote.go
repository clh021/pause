package app

import "pause/internal/remoteserver"

func (a *App) GetRemoteServerInfo() RemoteServerInfo {
	cfg := remoteserver.DefaultConfig()
	if a != nil {
		cfg = a.remoteServerConfig.Normalize()
	}
	return RemoteServerInfo{
		Enabled:       cfg.Enabled,
		Running:       a != nil && a.remoteServer != nil,
		LocalBaseURL:  cfg.LocalBaseURL(),
		AccessToken:   cfg.Token,
		TokenRequired: cfg.Token != "",
		LastError:     remoteServerLastError(a),
	}
}

func remoteServerLastError(a *App) string {
	if a == nil {
		return ""
	}
	return a.remoteServerLastErr
}
