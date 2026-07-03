package desktop

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"pause/internal/meta"
	"pause/internal/remoteserver"
)

var errHelpRequested = errors.New("help requested")

type LaunchOptions struct {
	Headless        bool
	Windowed        bool
	PrintRemoteInfo bool
}

func ResolveLaunchOptions(args []string) (LaunchOptions, error) {
	opts := LaunchOptions{}

	if strings.EqualFold(strings.TrimSpace(os.Getenv("PAUSE_HEADLESS")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("PAUSE_HEADLESS")), "true") {
		opts.Headless = true
	}

	for _, arg := range args {
		switch strings.TrimSpace(arg) {
		case "--headless":
			opts.Headless = true
			opts.Windowed = false
		case "--windowed", "--gui":
			opts.Headless = false
			opts.Windowed = true
		case "--print-remote-info":
			opts.PrintRemoteInfo = true
		case "-h", "--help":
			return LaunchOptions{}, errHelpRequested
		}
	}

	return opts, nil
}

func ShouldDefaultHeadlessForPlatform(goos string, opts LaunchOptions) bool {
	return goos == "windows" && !opts.Headless && !opts.Windowed && !opts.PrintRemoteInfo
}

func PrintRemoteInfo(w io.Writer) error {
	path, err := remoteserver.ConfigPath()
	if err != nil {
		return err
	}
	cfg, err := remoteserver.LoadConfig()
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, formatRemoteInfo(path, cfg, meta.CurrentBuildInfo()))
	return err
}

func formatRemoteInfo(path string, cfg remoteserver.Config, build meta.BuildInfo) string {
	return fmt.Sprintf(
		"version=%s\nbranch=%s\ncommit=%s\nbuild_time=%s\nvcs_modified=%s\nconfig=%s\nlocal_url=%s\ntoken=%s\nbind=%s:%d\nenabled=%t\n",
		build.Version,
		build.Branch,
		build.Commit,
		build.BuildTime,
		build.Modified,
		path,
		cfg.LocalBaseURL(),
		cfg.Token,
		cfg.BindAddress,
		cfg.Port,
		cfg.Enabled,
	)
}
