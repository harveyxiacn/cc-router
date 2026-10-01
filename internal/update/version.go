package update

import (
	"errors"
	"strings"
)

type semanticVersion struct {
	core [3]string
	pre  []string
}

func parseVersion(s string) (semanticVersion, error) {
	var v semanticVersion
	if len(s) > 128 {
		return v, errors.New("invalid semantic version")
	}
	s = strings.TrimPrefix(s, "v")
	main, build, hasBuild := strings.Cut(s, "+")
	if hasBuild && !validIdentifiers(build, false) {
		return v, errors.New("invalid semantic version")
	}
	core, pre, hasPre := strings.Cut(main, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, errors.New("invalid semantic version")
	}
	for i, p := range parts {
		if !numeric(p) || len(p) > 1 && p[0] == '0' {
			return v, errors.New("invalid semantic version")
		}
		v.core[i] = p
	}
	if hasPre {
		if !validIdentifiers(pre, true) {
			return v, errors.New("invalid semantic version")
		}
		v.pre = strings.Split(pre, ".")
	}
	return v, nil
}
func numeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func validIdentifiers(s string, prerelease bool) bool {
	for _, p := range strings.Split(s, ".") {
		if p == "" {
			return false
		}
		for _, c := range p {
			if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '-') {
				return false
			}
		}
		if prerelease && numeric(p) && len(p) > 1 && p[0] == '0' {
			return false
		}
	}
	return true
}
func compareNumeric(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}
func compareVersions(a, b semanticVersion) int {
	for i := range a.core {
		if c := compareNumeric(a.core[i], b.core[i]); c != 0 {
			return c
		}
	}
	if len(a.pre) == 0 && len(b.pre) == 0 {
		return 0
	}
	if len(a.pre) == 0 {
		return 1
	}
	if len(b.pre) == 0 {
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		x, y := a.pre[i], b.pre[i]
		xn, yn := numeric(x), numeric(y)
		var c int
		switch {
		case xn && yn:
			c = compareNumeric(x, y)
		case xn:
			c = -1
		case yn:
			c = 1
		default:
			c = strings.Compare(x, y)
		}
		if c != 0 {
			return c
		}
	}
	if len(a.pre) < len(b.pre) {
		return -1
	}
	if len(a.pre) > len(b.pre) {
		return 1
	}
	return 0
}

// IsNewer compares semantic versions, including prerelease identifiers. Build
// metadata does not change precedence. A release tag's optional v is accepted.
func IsNewer(candidate, current string) (bool, error) {
	a, err := parseVersion(candidate)
	if err != nil {
		return false, err
	}
	b, err := parseVersion(current)
	if err != nil {
		return false, err
	}
	return compareVersions(a, b) > 0, nil
}
