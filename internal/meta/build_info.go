package meta

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
)

const unknownBuildValue = "unknown"
const develBuildVersion = "(devel)"

// These values can be injected at build time, for example:
//
//	-ldflags "-X pause/internal/meta.Version=0.9.7 -X pause/internal/meta.GitBranch=main -X pause/internal/meta.GitCommit=abc123 -X pause/internal/meta.BuildTime=2026-07-03T12:00:00Z"
var (
	Version   string
	GitBranch string
	GitCommit string
	BuildTime string
)

type BuildInfo struct {
	Version   string
	Branch    string
	Commit    string
	BuildTime string
	Modified  string
}

func CurrentBuildInfo() BuildInfo {
	info := BuildInfo{
		Version:   unknownBuildValue,
		Branch:    unknownBuildValue,
		Commit:    unknownBuildValue,
		BuildTime: unknownBuildValue,
		Modified:  unknownBuildValue,
	}

	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimSpace(buildInfo.Main.Version); v != "" {
			info.Version = v
		}
		applyBuildSettings(&info, buildInfo.Settings)
	}

	applyRepoFallback(&info)

	overrideBuildField(&info.Version, Version)
	overrideBuildField(&info.Branch, GitBranch)
	overrideBuildField(&info.Commit, GitCommit)
	overrideBuildField(&info.BuildTime, BuildTime)

	return info
}

func applyBuildSettings(info *BuildInfo, settings []debug.BuildSetting) {
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.branch":
			overrideBuildField(&info.Branch, setting.Value)
		case "vcs.revision":
			overrideBuildField(&info.Commit, setting.Value)
		case "vcs.time":
			overrideBuildField(&info.BuildTime, setting.Value)
		case "vcs.modified":
			overrideBuildField(&info.Modified, setting.Value)
		}
	}
}

func overrideBuildField(dst *string, value string) {
	if v := strings.TrimSpace(value); v != "" {
		*dst = v
	}
}

func applyRepoFallback(info *BuildInfo) {
	root, ok := findProjectRoot()
	if !ok {
		return
	}

	if needsVersionFallback(info.Version) {
		overrideBuildField(&info.Version, readTrimmedFile(filepath.Join(root, "VERSION")))
	}
	if info.Branch == unknownBuildValue {
		branch, ok := gitOutput(root, "rev-parse", "--abbrev-ref", "HEAD")
		if ok {
			if branch == "HEAD" {
				branch = "detached"
			}
			overrideBuildField(&info.Branch, branch)
		}
	}
	if info.Commit == unknownBuildValue {
		if commit, ok := gitOutput(root, "rev-parse", "HEAD"); ok {
			overrideBuildField(&info.Commit, commit)
		}
	}
	if info.Modified == unknownBuildValue {
		if status, ok := gitOutput(root, "status", "--porcelain"); ok {
			if strings.TrimSpace(status) == "" {
				info.Modified = "false"
			} else {
				info.Modified = "true"
			}
		}
	}
}

func needsVersionFallback(version string) bool {
	version = strings.TrimSpace(version)
	return version == "" || version == unknownBuildValue || version == develBuildVersion
}

func readTrimmedFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func gitOutput(root string, args ...string) (string, bool) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func findProjectRoot() (string, bool) {
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		dir := candidate
		for {
			if _, ok := seen[dir]; ok {
				break
			}
			seen[dir] = struct{}{}
			if looksLikeProjectRoot(dir) {
				return dir, true
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", false
}

func looksLikeProjectRoot(dir string) bool {
	goModPath := filepath.Join(dir, "go.mod")
	versionPath := filepath.Join(dir, "VERSION")
	if _, err := os.Stat(versionPath); err != nil {
		return false
	}
	fields := strings.Fields(readTrimmedFile(goModPath))
	return len(fields) >= 2 && fields[0] == "module" && fields[1] == "pause"
}
