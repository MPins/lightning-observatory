package version

import (
	"runtime/debug"
	"testing"
)

func TestFromBuildInfo(t *testing.T) {
	const rev = "ffe0277e7ac76644e3c43595a47fc84dc876c056"

	vcs := func(modified string) []debug.BuildSetting {
		return []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: rev},
			{Key: "vcs.modified", Value: modified},
		}
	}

	tests := []struct {
		name     string
		version  string
		settings []debug.BuildSetting
		want     Info
		str      string
	}{
		{
			name:     "release tag",
			version:  "v0.1.0",
			settings: vcs("false"),
			want:     Info{Version: "v0.1.0", Revision: rev},
			str:      "v0.1.0 (ffe0277e7ac7)",
		},
		{
			name:     "untagged commit",
			version:  "v0.0.0-20261119224038-ffe0277e7ac7",
			settings: vcs("false"),
			want: Info{
				Version:  "v0.0.0-20261119224038-ffe0277e7ac7",
				Revision: rev,
			},
			str: "v0.0.0-20261119224038-ffe0277e7ac7 (ffe0277e7ac7)",
		},
		{
			name:     "dirty tree",
			version:  "v0.1.0+dirty",
			settings: vcs("true"),
			want: Info{
				Version:  "v0.1.0+dirty",
				Revision: rev,
				Modified: true,
			},
			str: "v0.1.0+dirty (ffe0277e7ac7, modified)",
		},
		{
			name:    "go run without VCS",
			version: "(devel)",
			want:    Info{Version: "devel"},
			str:     "devel",
		},
		{
			name: "no version at all",
			want: Info{Version: "devel"},
			str:  "devel",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bi := &debug.BuildInfo{
				Main:     debug.Module{Version: tc.version},
				Settings: tc.settings,
			}
			got := FromBuildInfo(bi)
			if got != tc.want {
				t.Fatalf("FromBuildInfo() = %+v, want %+v", got, tc.want)
			}
			if s := got.String(); s != tc.str {
				t.Fatalf("String() = %q, want %q", s, tc.str)
			}
		})
	}
}
