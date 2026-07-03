//go:build !wails

package main

import (
	"os"

	entry "pause/internal/entry/desktop"
	"pause/internal/logx"
)

func main() {
	opts, err := entry.ResolveLaunchOptions(os.Args[1:])
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

	if opts.PrintRemoteInfo {
		if err = entry.PrintRemoteInfo(os.Stdout); err != nil {
			logx.Errorf("failed to print remote info: %v", err)
			os.Exit(1)
		}
	}

	if opts.Headless || !opts.PrintRemoteInfo {
		err = entry.RunHeadless("")
	}

	if err != nil {
		logx.Errorf("failed to init app: %v", err)
		os.Exit(1)
	}
}
