//go:build wails

package main

import (
	"embed"
	"os"

	entry "pause/internal/entry/desktop"
	"pause/internal/logx"
)

//go:embed all:frontend/dist
var bundledAssets embed.FS

func main() {
	action, err := entry.ResolveLaunchAction(os.Args[1:])
	if err != nil {
		if err.Error() == "help requested" {
			if _, printErr := os.Stdout.WriteString("Usage:\n  Pause [--headless|--print-remote-info]\n"); printErr != nil {
				os.Exit(1)
			}
			return
		}
		logx.Errorf("failed to resolve launch mode: %v", err)
		os.Exit(1)
	}

	switch action {
	case entry.LaunchHeadless:
		err = entry.RunHeadless("")
	case entry.LaunchPrintRemoteInfo:
		err = entry.PrintRemoteInfo(os.Stdout)
	default:
		err = entry.RunWailsFromEmbedded("", bundledAssets, "frontend/dist")
	}

	if err != nil {
		logx.Errorf("failed to init app: %v", err)
		os.Exit(1)
	}
}
