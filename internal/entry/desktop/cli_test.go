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
			name: "print and headless",
			args: []string{"--print-remote-info", "--headless"},
			want: LaunchOptions{Headless: true, PrintRemoteInfo: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
