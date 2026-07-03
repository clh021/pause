package desktop

import (
	"strings"
	"testing"

	"pause/internal/meta"
	"pause/internal/remoteserver"
)

func TestResolveLaunchOptions_Table(t *testing.T) {
	t.Setenv("PAUSE_HEADLESS", "")

	cases := []struct {
		name string
		args []string
		want LaunchOptions
	}{
		{
			name: "default gui",
			args: nil,
			want: LaunchOptions{},
		},
		{
			name: "headless only",
			args: []string{"--headless"},
			want: LaunchOptions{Headless: true},
		},
		{
			name: "print only",
			args: []string{"--print-remote-info"},
			want: LaunchOptions{PrintRemoteInfo: true},
		},
		{
			name: "windowed only",
			args: []string{"--windowed"},
			want: LaunchOptions{Windowed: true},
		},
		{
			name: "gui alias",
			args: []string{"--gui"},
			want: LaunchOptions{Windowed: true},
		},
		{
			name: "print and headless",
			args: []string{"--print-remote-info", "--headless"},
			want: LaunchOptions{Headless: true, PrintRemoteInfo: true},
		},
		{
			name: "windowed overrides env headless",
			args: []string{"--windowed"},
			want: LaunchOptions{Windowed: true},
		},
		{
			name: "headless wins after windowed",
			args: []string{"--windowed", "--headless"},
			want: LaunchOptions{Headless: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "windowed overrides env headless" {
				t.Setenv("PAUSE_HEADLESS", "true")
			} else {
				t.Setenv("PAUSE_HEADLESS", "")
			}
			got, err := ResolveLaunchOptions(tc.args)
			if err != nil {
				t.Fatalf("ResolveLaunchOptions() err=%v", err)
			}
			if got != tc.want {
				t.Fatalf("ResolveLaunchOptions()=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestResolveLaunchOptions_HeadlessFromEnv(t *testing.T) {
	t.Setenv("PAUSE_HEADLESS", "true")

	got, err := ResolveLaunchOptions(nil)
	if err != nil {
		t.Fatalf("ResolveLaunchOptions() err=%v", err)
	}
	if !got.Headless {
		t.Fatalf("expected headless=true, got %+v", got)
	}
}

func TestShouldDefaultHeadlessForPlatform(t *testing.T) {
	cases := []struct {
		name string
		goos string
		opts LaunchOptions
		want bool
	}{
		{
			name: "windows default",
			goos: "windows",
			opts: LaunchOptions{},
			want: true,
		},
		{
			name: "linux default",
			goos: "linux",
			opts: LaunchOptions{},
			want: false,
		},
		{
			name: "explicit headless not defaulted",
			goos: "windows",
			opts: LaunchOptions{Headless: true},
			want: false,
		},
		{
			name: "windowed disables default headless",
			goos: "windows",
			opts: LaunchOptions{Windowed: true},
			want: false,
		},
		{
			name: "print only disables default headless",
			goos: "windows",
			opts: LaunchOptions{PrintRemoteInfo: true},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldDefaultHeadlessForPlatform(tc.goos, tc.opts); got != tc.want {
				t.Fatalf("ShouldDefaultHeadlessForPlatform(%q, %+v)=%t want=%t", tc.goos, tc.opts, got, tc.want)
			}
		})
	}
}

func TestFormatRemoteInfo_IncludesBuildMetadata(t *testing.T) {
	cfg := remoteserver.Config{
		Enabled:     true,
		BindAddress: "0.0.0.0",
		Port:        18680,
		Token:       "secret-token",
	}
	build := meta.BuildInfo{
		Version:   "0.9.7",
		Branch:    "webCtrl",
		Commit:    "deadbeef",
		BuildTime: "2026-07-03T12:34:56Z",
		Modified:  "false",
	}

	output := formatRemoteInfo("/tmp/remote_server.json", cfg, build)

	for _, want := range []string{
		"version=0.9.7",
		"branch=webCtrl",
		"commit=deadbeef",
		"build_time=2026-07-03T12:34:56Z",
		"vcs_modified=false",
		"config=/tmp/remote_server.json",
		"local_url=http://127.0.0.1:18680",
		"token=secret-token",
		"bind=0.0.0.0:18680",
		"enabled=true",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}
