package cli

import (
	"runtime/debug"
	"strings"
)

// init fills Version/Commit/Date from Go's embedded build info when our
// ldflags weren't applied (typically when the binary was built via
// `go install github.com/.../cmd/kcm@v0.9.1` rather than our scripts/build.sh
// or goreleaser pipeline). The ldflag path still wins when it sets anything
// other than the compile-time defaults, so released builds keep their
// reproducible values.
func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	if Version == "dev" {
		if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
			Version = strings.TrimPrefix(v, "v")
		}
	}

	if Commit == "none" {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				Commit = s.Value
				if len(Commit) > 7 {
					Commit = Commit[:7]
				}
				break
			}
		}
	}

	if Date == "unknown" {
		for _, s := range info.Settings {
			if s.Key == "vcs.time" && s.Value != "" {
				Date = s.Value
				break
			}
		}
	}
}
