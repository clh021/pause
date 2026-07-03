package desktop

import "testing"

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
