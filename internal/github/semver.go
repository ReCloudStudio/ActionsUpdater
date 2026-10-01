package github

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var semver = regexp.MustCompile(`^v?(0|[1-9]\d*)(?:\.(0|[1-9]\d*))?(?:\.(0|[1-9]\d*))?(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

type Version struct {
	Major, Minor, Patch int
	Parts               int
	Pre                 string
	Name                string
}

func ParseVersion(s string) (Version, bool) {
	m := semver.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
	parts := 1
	if m[2] != "" {
		parts = 2
	}
	if m[3] != "" {
		parts = 3
	}
	return Version{n(1), n(2), n(3), parts, m[4], s}, true
}
func (v Version) Stable() bool { return v.Pre == "" }
func Compare(a, b Version) int {
	for _, p := range [][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if p[0] > p[1] {
			return 1
		}
		if p[0] < p[1] {
			return -1
		}
	}
	if a.Pre == b.Pre {
		return 0
	}
	if a.Pre == "" {
		return 1
	}
	if b.Pre == "" {
		return -1
	}
	ap, bp := strings.Split(a.Pre, "."), strings.Split(b.Pre, ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		if ap[i] == bp[i] {
			continue
		}
		ai, ae := strconv.Atoi(ap[i])
		bi, be := strconv.Atoi(bp[i])
		if ae == nil && be == nil {
			if ai > bi {
				return 1
			}
			return -1
		}
		if ae == nil {
			return -1
		}
		if be == nil {
			return 1
		}
		if ap[i] > bp[i] {
			return 1
		}
		return -1
	}
	if len(ap) > len(bp) {
		return 1
	}
	return -1
}
func Select(tags []Tag, current string, sameMajor, includePre bool) (Tag, bool) {
	cur, curOK := ParseVersion(current)
	hasSemver := false
	candidates := []struct {
		tag Tag
		v   Version
	}{}
	for _, t := range tags {
		v, ok := ParseVersion(t.Name)
		if !ok {
			continue
		}
		hasSemver = true
		if sameMajor && (!curOK || v.Major != cur.Major) {
			continue
		}
		if curOK && cur.Pre != "" {
			if v.Pre == "" || !samePreReleaseFamily(cur.Pre, v.Pre) {
				continue
			}
		}
		if !includePre && v.Pre != "" && (!curOK || cur.Pre == "") {
			continue
		}
		candidates = append(candidates, struct {
			tag Tag
			v   Version
		}{t, v})
	}
	if len(candidates) > 0 {
		sort.SliceStable(candidates, func(i, j int) bool {
			c := Compare(candidates[i].v, candidates[j].v)
			if c != 0 {
				return c > 0
			}
			if candidates[i].v.Parts != candidates[j].v.Parts {
				return candidates[i].v.Parts > candidates[j].v.Parts
			}
			return candidates[i].tag.Name < candidates[j].tag.Name
		})
		return candidates[0].tag, true
	}
	if hasSemver || sameMajor && !curOK {
		return Tag{}, false
	}
	if len(tags) == 0 {
		return Tag{}, false
	}
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].Time.Equal(tags[j].Time) {
			return tags[i].Name < tags[j].Name
		}
		return tags[i].Time.After(tags[j].Time)
	})
	return tags[0], true
}

func samePreReleaseFamily(a, b string) bool {
	return strings.Split(a, ".")[0] == strings.Split(b, ".")[0]
}
