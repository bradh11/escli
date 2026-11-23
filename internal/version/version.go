// Package version holds the build information that the release build sets with -ldflags.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// These values are set at build time, for example:
//
//	go build -ldflags "-X github.com/bradh11/escli/internal/version.Version=v1.2.3"
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns the version line that `escli version` and `escli --version` print.
func String() string {
	v, c := Version, Commit
	// A binary installed with `go install module@version` has no ldflags,
	// but the Go toolchain records the module version and VCS revision.
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if mv := info.Main.Version; mv != "" && mv != "(devel)" {
				v = mv
			}
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && c == "none" && len(s.Value) >= 7 {
					c = s.Value[:7]
				}
			}
		}
	}
	return fmt.Sprintf("%s (commit %s, built %s, %s/%s)", v, c, Date, runtime.GOOS, runtime.GOARCH)
}
