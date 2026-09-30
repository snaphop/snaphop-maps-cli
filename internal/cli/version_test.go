package cli

import (
	"runtime/debug"
	"testing"
)

func TestVersionResolution(t *testing.T) {
	t.Parallel()
	info := func(version string, ok bool) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: version}}, ok
		}
	}
	cases := []struct {
		stamped string
		info    func() (*debug.BuildInfo, bool)
		want    string
	}{
		{"v1.0.0", info("v0.9.0", true), "v1.0.0"},
		{"dev", info("v0.9.0", true), "v0.9.0"},
		{"", info("v0.9.0", true), "v0.9.0"},
		{"dev", info("(devel)", true), "dev"},
		{"dev", info("", true), "dev"},
		{"dev", info("v0.9.0", false), "dev"},
	}
	for _, tc := range cases {
		if got := Version(tc.stamped, tc.info); got != tc.want {
			t.Errorf("Version(%q) = %q, want %q", tc.stamped, got, tc.want)
		}
	}
}
