package internal

import (
	"runtime/debug"
	"testing"
)

func TestFillVersionFromBuildInfo(t *testing.T) {
	for name, tc := range map[string]struct {
		ldflagsVersion string
		info           *debug.BuildInfo
		expVersion     string
		expCommit      string
	}{
		"go install records module version": {
			ldflagsVersion: "dev",
			info:           &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}},
			expVersion:     "v0.1.0",
			expCommit:      "none",
		},
		"local build uses vcs revision": {
			ldflagsVersion: "dev",
			info: &debug.BuildInfo{
				Main:     debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "0123456789abcdef0123"}},
			},
			expVersion: "dev",
			expCommit:  "0123456789ab",
		},
		"ldflags win": {
			ldflagsVersion: "v0.2.0",
			info:           &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}},
			expVersion:     "v0.2.0",
			expCommit:      "none",
		},
	} {
		t.Run(name, func(t *testing.T) {
			origVersion, origCommit, origTime := BuildVersion, GitCommit, BuildTime
			t.Cleanup(func() { BuildVersion, GitCommit, BuildTime = origVersion, origCommit, origTime })
			BuildVersion, GitCommit, BuildTime = tc.ldflagsVersion, "none", "unknown"
			fillVersionFromBuildInfo(func() (*debug.BuildInfo, bool) { return tc.info, true })
			if BuildVersion != tc.expVersion || GitCommit != tc.expCommit {
				t.Fatalf("got version %q commit %q, want %q %q", BuildVersion, GitCommit, tc.expVersion, tc.expCommit)
			}
		})
	}
}
