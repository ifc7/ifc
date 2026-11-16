package internal

import "runtime/debug"

func init() {
	fillVersionFromBuildInfo(debug.ReadBuildInfo)
}

// fillVersionFromBuildInfo fills in BuildVersion and GitCommit when they were not set
// with -ldflags, as with `go install github.com/ifc7/ifc/cmd/ifc@<version>`, which
// records the module version and, for local builds, the VCS revision.
func fillVersionFromBuildInfo(read func() (*debug.BuildInfo, bool)) {
	info, ok := read()
	if !ok || info == nil {
		return
	}
	if BuildVersion == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		BuildVersion = info.Main.Version
	}
	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision" && GitCommit == "none" && s.Value != "":
			GitCommit = s.Value
			if len(GitCommit) > 12 {
				GitCommit = GitCommit[:12]
			}
		case s.Key == "vcs.time" && BuildTime == "unknown" && s.Value != "":
			BuildTime = s.Value
		}
	}
}
