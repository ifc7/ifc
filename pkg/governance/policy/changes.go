package policy

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// ChangeSettings are an organization's per-kind levels for structured
// change records. A kind not listed keeps the level its compare plugin gave
// it: error for a breaking change, warn otherwise.
type ChangeSettings struct {
	// Kinds maps a change kind (for example "schema.properties.added") to
	// error, warn or ignore. error makes the kind count as breaking for
	// gates, warn reports it without failing a gate, ignore drops it.
	Kinds map[string]contract.ChangeLevel `json:"kinds,omitempty"`
}

// MaxChangeKinds bounds the kinds one policy may set.
const MaxChangeKinds = 200

var changeKindRe = regexp.MustCompile(`^[A-Za-z]+\.[A-Za-z0-9$*-]+\.(added|removed|modified)$`)

// Validate checks every kind's format and level.
func (c ChangeSettings) Validate() error {
	if len(c.Kinds) > MaxChangeKinds {
		return fmt.Errorf("%w: at most %d change kinds", ErrInvalid, MaxChangeKinds)
	}
	for k, l := range c.Kinds {
		if !changeKindRe.MatchString(k) {
			return fmt.Errorf("%w: change kind %q must look like object.property.added|removed|modified", ErrInvalid, k)
		}
		switch l {
		case contract.ChangeLevelError, contract.ChangeLevelWarn, contract.ChangeLevelIgnore:
		default:
			return fmt.Errorf("%w: change kind %q level must be error, warn or ignore", ErrInvalid, k)
		}
	}
	return nil
}

// Apply returns cs with each change's level set from the settings and
// ignored kinds dropped. A change exempted by x-stability-level stays at
// warn: an unstable operation may change freely whatever its kind.
func (c ChangeSettings) Apply(cs []contract.Change) []contract.Change {
	out := make([]contract.Change, 0, len(cs))
	for _, ch := range cs {
		if l, ok := c.Kinds[ch.Kind]; ok {
			if l == contract.ChangeLevelIgnore {
				continue
			}
			if ch.Stability == "" {
				ch.Level = l
			}
		}
		out = append(out, ch)
	}
	return out
}

// Version is a parsed release version: major.minor.patch with an optional
// leading v and optional pre-release or build suffix (ignored here).
type Version struct {
	Major, Minor, Patch int
}

// ParseVersion parses a release version such as 1.2.3 or v1.2.3-rc.1.
func ParseVersion(s string) (Version, error) {
	core := strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("%w: version %q is not major.minor.patch", ErrInvalid, s)
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return Version{}, fmt.Errorf("%w: version %q is not major.minor.patch", ErrInvalid, s)
		}
		n[i] = v
	}
	return Version{Major: n[0], Minor: n[1], Patch: n[2]}, nil
}

// AllowsBreaking reports whether moving from `from` to `to` is a bump that
// may carry a breaking change: a major bump, or for a 0.x version a minor
// (or major) bump, as semantic versioning treats 0.x.
func AllowsBreaking(from, to Version) bool {
	if from.Major == 0 {
		return to.Major > 0 || to.Minor > from.Minor
	}
	return to.Major > from.Major
}

// breakingChanges returns the changes at level error.
func breakingChanges(cs []contract.Change) []contract.Change {
	return slices.DeleteFunc(slices.Clone(cs), func(c contract.Change) bool { return c.Level != contract.ChangeLevelError })
}
