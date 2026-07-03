package meta

import (
	"runtime/debug"
	"testing"
)

func TestApplyBuildSettings(t *testing.T) {
	info := BuildInfo{
		Version:   unknownBuildValue,
		Branch:    unknownBuildValue,
		Commit:    unknownBuildValue,
		BuildTime: unknownBuildValue,
		Modified:  unknownBuildValue,
	}

	applyBuildSettings(&info, []debug.BuildSetting{
		{Key: "vcs.branch", Value: "webCtrl"},
		{Key: "vcs.revision", Value: "abc123"},
		{Key: "vcs.time", Value: "2026-07-03T10:11:12Z"},
		{Key: "vcs.modified", Value: "true"},
	})

	if info.Branch != "webCtrl" {
		t.Fatalf("Branch=%q want %q", info.Branch, "webCtrl")
	}
	if info.Commit != "abc123" {
		t.Fatalf("Commit=%q want %q", info.Commit, "abc123")
	}
	if info.BuildTime != "2026-07-03T10:11:12Z" {
		t.Fatalf("BuildTime=%q want %q", info.BuildTime, "2026-07-03T10:11:12Z")
	}
	if info.Modified != "true" {
		t.Fatalf("Modified=%q want %q", info.Modified, "true")
	}
}

func TestCurrentBuildInfo_OverridesWin(t *testing.T) {
	origVersion := Version
	origBranch := GitBranch
	origCommit := GitCommit
	origBuildTime := BuildTime
	t.Cleanup(func() {
		Version = origVersion
		GitBranch = origBranch
		GitCommit = origCommit
		BuildTime = origBuildTime
	})

	Version = "0.9.7"
	GitBranch = "webCtrl"
	GitCommit = "deadbeef"
	BuildTime = "2026-07-03T12:34:56Z"

	info := CurrentBuildInfo()

	if info.Version != "0.9.7" {
		t.Fatalf("Version=%q want %q", info.Version, "0.9.7")
	}
	if info.Branch != "webCtrl" {
		t.Fatalf("Branch=%q want %q", info.Branch, "webCtrl")
	}
	if info.Commit != "deadbeef" {
		t.Fatalf("Commit=%q want %q", info.Commit, "deadbeef")
	}
	if info.BuildTime != "2026-07-03T12:34:56Z" {
		t.Fatalf("BuildTime=%q want %q", info.BuildTime, "2026-07-03T12:34:56Z")
	}
}
