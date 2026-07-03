//go:build !wails

package main

import (
	"os"

	entry "pause/internal/entry/desktop"
	"pause/internal/logx"
)

func main() {
	action, err := entry.ResolveLaunchAction(os.Args[1:])
	if err != nil {
		if err.Error() == "help requested" {
			if _, printErr := os.Stdout.WriteString("Usage:\n  pause [--headless|--print-remote-info]\n"); printErr != nil {
				os.Exit(1)
			}
			return
		}
		logx.Errorf("failed to resolve launch mode: %v", err)
		os.Exit(1)
	}

	switch action {
	case entry.LaunchPrintRemoteInfo:
		err = entry.PrintRemoteInfo(os.Stdout)
	default:
		err = entry.RunHeadless("")
	}

	if err != nil {
		logx.Errorf("failed to init app: %v", err)
		os.Exit(1)
	}
}
