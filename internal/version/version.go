// Package version carries build identity.
//
// The binary previously had no version string anywhere: no --version flag, no
// version in /health, nothing in the UI, and zero git tags across the repo's
// history. On a fleet there was no way to tell what was deployed.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Injected at build time:
//
//	go build -ldflags "-X sys-sentient/internal/version.Version=v0.1.0 \
//	                   -X sys-sentient/internal/version.Commit=$(git rev-parse --short HEAD) \
//	                   -X sys-sentient/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	Version   = "dev"
	Commit    = ""
	BuildDate = ""
)

// Info describes this build.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns build identity, falling back to the VCS stamps the Go toolchain
// embeds automatically when ldflags were not supplied.
func Get() Info {
	commit := Commit
	if commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range bi.Settings {
				if setting.Key == "vcs.revision" {
					commit = setting.Value
					if len(commit) > 12 {
						commit = commit[:12]
					}
				}
			}
		}
	}

	return Info{
		Version:   Version,
		Commit:    commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String renders a one-line summary for --version.
func (i Info) String() string {
	s := "sys-sentient " + i.Version
	if i.Commit != "" {
		s += " (" + i.Commit + ")"
	}
	if i.BuildDate != "" {
		s += " built " + i.BuildDate
	}
	return s + " " + i.GoVersion + " " + i.Platform
}
