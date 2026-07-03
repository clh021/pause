//go:build wails

package main

import (
	"embed"
	"os"
	"runtime"

	entry "pause/internal/entry/desktop"
	"pause/internal/logx"
)

//go:embed all:frontend/dist
var bundledAssets embed.FS

func main() {
	opts, err := entry.ResolveLaunchOptions(os.Args[1:])
	if err != nil {
		if err.Error() == "help requested" {
			if _, printErr := os.Stdout.WriteString("Usage:\n  Pause [--headless|--windowed|--gui|--print-remote-info]\n"); printErr != nil {
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

	shouldDefaultHeadless := entry.ShouldDefaultHeadlessForPlatform(runtime.GOOS, opts)

	switch {
	case opts.Headless || shouldDefaultHeadless:
		err = entry.RunHeadless("")
	case opts.PrintRemoteInfo:
		err = nil
	default:
		err = entry.RunWailsFromEmbedded("", bundledAssets, "frontend/dist")
	}

	if err != nil {
		logx.Errorf("failed to init app: %v", err)
		os.Exit(1)
	}
}
