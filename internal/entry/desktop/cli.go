package desktop

import (
	"fmt"
	"io"
	"os"
	"strings"

	"pause/internal/remoteserver"
)

type LaunchAction int

const (
	LaunchGUI LaunchAction = iota
	LaunchHeadless
	LaunchPrintRemoteInfo
)

func ResolveLaunchAction(args []string) (LaunchAction, error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("PAUSE_HEADLESS")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("PAUSE_HEADLESS")), "true") {
		return LaunchHeadless, nil
	}

	for _, arg := range args {
		switch strings.TrimSpace(arg) {
		case "--headless":
			return LaunchHeadless, nil
		case "--print-remote-info":
			return LaunchPrintRemoteInfo, nil
		case "-h", "--help":
			return LaunchPrintRemoteInfo, fmt.Errorf("help requested")
		}
	}

	return LaunchGUI, nil
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
