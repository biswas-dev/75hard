// Package version carries build metadata injected at link time.
package version

// Set with -ldflags "-X .../internal/version.Version=1.0.0" at build time.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

// Info is the payload served by /api/version.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildTime string `json:"build_time"`
}

// Get returns the current build information.
func Get() Info {
	return Info{Version: Version, GitCommit: GitCommit, BuildTime: BuildTime}
}
