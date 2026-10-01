package github

import (
	"testing"
	"time"
)

func TestSelect(t *testing.T) {
	tags := []Tag{{Name: "v1"}, {Name: "v1.2"}, {Name: "v1.2.3"}, {Name: "v2.0.0"}, {Name: "v3.0.0-rc.1"}}
	for _, tc := range []struct {
		name, current, want   string
		sameMajor, prerelease bool
	}{
		{"stable", "v1", "v2.0.0", false, false},
		{"same major", "v1", "v1.2.3", true, false},
		{"pre-release excluded", "v2", "v2.0.0", false, false},
		{"pre-release enabled", "v2", "v3.0.0-rc.1", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Select(tags, tc.current, tc.sameMajor, tc.prerelease)
			if !ok || got.Name != tc.want {
				t.Fatalf("Select() = %q, %v; want %q", got.Name, ok, tc.want)
			}
		})
	}
}

func TestSelectFallsBackToTime(t *testing.T) {
	tags := []Tag{{Name: "older", Time: time.Unix(1, 0)}, {Name: "newer", Time: time.Unix(2, 0)}}
	got, ok := Select(tags, "main", false, false)
	if !ok || got.Name != "newer" {
		t.Fatalf("Select() = %q, %v", got.Name, ok)
	}
}

func TestSelectKeepsPreReleaseFamily(t *testing.T) {
	tags := []Tag{{Name: "v2.0.0-beta.2"}, {Name: "v2.0.0-rc.1"}, {Name: "v2.0.0-beta.3"}}
	got, ok := Select(tags, "v2.0.0-beta.1", false, false)
	if !ok || got.Name != "v2.0.0-beta.3" {
		t.Fatalf("Select() = %q, %v", got.Name, ok)
	}
}
