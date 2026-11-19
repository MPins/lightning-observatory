// Package version reports which build of the observatory is running, from
// the information the Go toolchain embeds in every binary.
package version

import (
	"fmt"
	"runtime/debug"
)

// Info identifies a build.
type Info struct {
	// Version is the module version: a release tag such as "v0.1.0", a
	// pseudo-version for an untagged commit, or "devel" when unknown.
	Version string

	// Revision is the full commit hash, empty when the build had no VCS
	// information.
	Revision string

	// Modified is true when the tree had uncommitted changes.
	Modified bool
}

// Get returns the build information of the running binary.
func Get() Info {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return Info{Version: "devel"}
	}
	return FromBuildInfo(bi)
}

// FromBuildInfo extracts the build identity from bi.
func FromBuildInfo(bi *debug.BuildInfo) Info {
	info := Info{Version: bi.Main.Version}
	if info.Version == "" || info.Version == "(devel)" {
		info.Version = "devel"
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}

// String formats the build as "v0.1.0 (ffe0277e7ac7)", with ", modified"
// when the tree was dirty.
func (i Info) String() string {
	if i.Revision == "" {
		return i.Version
	}
	rev := i.Revision
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if i.Modified {
		return fmt.Sprintf("%s (%s, modified)", i.Version, rev)
	}
	return fmt.Sprintf("%s (%s)", i.Version, rev)
}
