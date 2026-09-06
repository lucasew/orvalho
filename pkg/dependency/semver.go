package dependency

import (
	"fmt"
	"strconv"
	"strings"
)

type version struct {
	major, minor, patch int
	pre                 string
}

func parseVersion(s string) (version, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return version{}, fmt.Errorf("%w: %q", ErrVersion, s)
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return version{}, fmt.Errorf("%w: %q: %w", ErrVersion, s, err)
		}
		if n < 0 {
			return version{}, fmt.Errorf("%w: %q", ErrVersion, s)
		}
		nums[i] = n
	}
	return version{nums[0], nums[1], nums[2], pre}, nil
}

func (v version) cmp(o version) int {
	if v.major != o.major {
		return v.major - o.major
	}
	if v.minor != o.minor {
		return v.minor - o.minor
	}
	if v.patch != o.patch {
		return v.patch - o.patch
	}
	if v.pre == o.pre {
		return 0
	}
	if v.pre == "" {
		return 1
	}
	if o.pre == "" {
		return -1
	}
	if v.pre < o.pre {
		return -1
	}
	return 1
}

func (v version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
	if v.pre != "" {
		s += "-" + v.pre
	}
	return s
}

// Satisfies reports whether ver is in the npm range (caret, tilde, exact, >=, *, x, latest).
func Satisfies(ver, rng string) bool {
	rng = strings.TrimSpace(rng)
	if rng == "" || rng == "*" || rng == "x" || rng == "latest" || rng == "x.x.x" {
		return !strings.Contains(ver, "-")
	}
	v, err := parseVersion(ver)
	if err != nil {
		return false
	}
	if strings.Contains(rng, "||") {
		for part := range strings.SplitSeq(rng, "||") {
			if Satisfies(ver, strings.TrimSpace(part)) {
				return true
			}
		}
		return false
	}
	if a, b, ok := strings.Cut(rng, " - "); ok {
		lo, err1 := parseVersion(strings.TrimSpace(a))
		hi, err2 := parseVersion(strings.TrimSpace(b))
		return err1 == nil && err2 == nil && v.cmp(lo) >= 0 && v.cmp(hi) <= 0
	}
	switch {
	case strings.HasPrefix(rng, "^"):
		base, err := parseVersion(strings.TrimPrefix(rng, "^"))
		if err != nil {
			return false
		}
		if v.cmp(base) < 0 {
			return false
		}
		var upper version
		if base.major > 0 {
			upper = version{base.major + 1, 0, 0, ""}
		} else if base.minor > 0 {
			upper = version{0, base.minor + 1, 0, ""}
		} else {
			upper = version{0, 0, base.patch + 1, ""}
		}
		return v.cmp(upper) < 0
	case strings.HasPrefix(rng, "~"):
		base, err := parseVersion(strings.TrimPrefix(rng, "~"))
		if err != nil {
			return false
		}
		if v.cmp(base) < 0 {
			return false
		}
		upper := version{base.major, base.minor + 1, 0, ""}
		return v.cmp(upper) < 0
	case strings.HasPrefix(rng, ">="):
		base, err := parseVersion(strings.TrimSpace(rng[2:]))
		return err == nil && v.cmp(base) >= 0
	case strings.HasPrefix(rng, ">"):
		base, err := parseVersion(strings.TrimSpace(rng[1:]))
		return err == nil && v.cmp(base) > 0
	case strings.HasPrefix(rng, "<="):
		base, err := parseVersion(strings.TrimSpace(rng[2:]))
		return err == nil && v.cmp(base) <= 0
	case strings.HasPrefix(rng, "<"):
		base, err := parseVersion(strings.TrimSpace(rng[1:]))
		return err == nil && v.cmp(base) < 0
	case strings.HasPrefix(rng, "="):
		base, err := parseVersion(strings.TrimSpace(rng[1:]))
		return err == nil && v.cmp(base) == 0
	default:
		base, err := parseVersion(rng)
		return err == nil && v.cmp(base) == 0
	}
}

func pickLatest(versions []string, rng string) (string, bool) {
	var best string
	var bestV version
	found := false
	for _, ver := range versions {
		if !Satisfies(ver, rng) {
			continue
		}
		v, err := parseVersion(ver)
		if err != nil {
			continue
		}
		if !found || v.cmp(bestV) > 0 {
			best, bestV, found = ver, v, true
		}
	}
	return best, found
}
