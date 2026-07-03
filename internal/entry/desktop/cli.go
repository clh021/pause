package desktop

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"pause/internal/remoteserver"
)

var errHelpRequested = errors.New("help requested")

type LaunchOptions struct {
	Headless        bool
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
		case "--print-remote-info":
			opts.PrintRemoteInfo = true
		case "-h", "--help":
			return LaunchOptions{}, errHelpRequested
		}
	}

	return opts, nil
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

	_, err = fmt.Fprintf(
		w,
		"config=%s\nlocal_url=%s\ntoken=%s\nbind=%s:%d\nenabled=%t\n",
		path,
		cfg.LocalBaseURL(),
		cfg.Token,
		cfg.BindAddress,
		cfg.Port,
		cfg.Enabled,
	)
	return err
}
