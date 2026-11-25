package ruleset

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// RefHost is the host of ifc7 ruleset references.
const RefHost = "ifc7.dev"

// ErrUnpinned is returned (wrapped) when an extends entry names an ifc7
// ruleset without a version. Effective rulesets must be reproducible, so
// every hub ruleset in an extends chain names one immutable version.
var ErrUnpinned = errors.New("ruleset reference must name a version")

// Ref is a reference to a ruleset on the ifc7 hub:
// https://ifc7.dev/r/{owner}/{slug}/v{n}. Version 0 means unpinned (the
// latest version), which the locator serves but extends does not accept.
type Ref struct {
	Owner   string
	Slug    string
	Version int
}

// Pinned reports whether r names a version.
func (r Ref) Pinned() bool { return r.Version > 0 }

// String returns the canonical URL form of r.
func (r Ref) String() string {
	s := "https://" + RefHost + "/r/" + r.Owner + "/" + r.Slug
	if r.Version > 0 {
		s += "/v" + strconv.Itoa(r.Version)
	}
	return s
}

// WithVersion returns r pinned to version n.
func (r Ref) WithVersion(n int) Ref {
	r.Version = n
	return r
}

var refSegment = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,253}[a-z0-9])?$`)

// IsRefName reports whether s is a valid owner or slug segment: lowercase
// letters, digits and inner hyphens, at most 255 characters.
func IsRefName(s string) bool { return refSegment.MatchString(s) }

// ParseRef parses an ifc7 ruleset reference. It accepts
// https://ifc7.dev/r/{owner}/{slug}[/v{n}] (http and a trailing slash are
// tolerated) and reports ok=false for anything that does not name the ifc7
// host and /r/ path, so callers can tell "not an ifc7 reference" from "a
// malformed one" (err != nil).
func ParseRef(s string) (ref Ref, ok bool, err error) {
	u, perr := url.Parse(strings.TrimSpace(s))
	if perr != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.EqualFold(u.Host, RefHost) {
		return Ref{}, false, nil
	}
	path := strings.TrimSuffix(u.Path, "/")
	rest, found := strings.CutPrefix(path, "/r/")
	if !found {
		return Ref{}, false, nil
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return Ref{}, true, fmt.Errorf("%w: ruleset reference %q must not have a query, fragment or credentials", ErrInvalid, s)
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return Ref{}, true, fmt.Errorf("%w: ruleset reference %q must look like https://%s/r/{owner}/{slug}/v{n}", ErrInvalid, s, RefHost)
	}
	ref = Ref{Owner: parts[0], Slug: parts[1]}
	if !IsRefName(ref.Owner) || !IsRefName(ref.Slug) {
		return Ref{}, true, fmt.Errorf("%w: ruleset reference %q has an invalid owner or slug", ErrInvalid, s)
	}
	if len(parts) == 3 {
		n, cerr := strconv.Atoi(strings.TrimPrefix(parts[2], "v"))
		if !strings.HasPrefix(parts[2], "v") || cerr != nil || n < 1 || strconv.Itoa(n) != parts[2][1:] {
			return Ref{}, true, fmt.Errorf("%w: ruleset reference %q has an invalid version (v1, v2, ...)", ErrInvalid, s)
		}
		ref.Version = n
	}
	return ref, true, nil
}

// RefResolver adapts a loader of ifc7 ruleset versions to Resolver. It
// accepts only pinned ifc7 references, so a resolver built with it never
// loads a local file or an arbitrary URL. The loader receives the parsed
// reference.
func RefResolver(load func(ctx context.Context, ref Ref) ([]byte, error)) Resolver {
	return ResolverFunc(func(ctx context.Context, name string) ([]byte, error) {
		ref, ok, err := ParseRef(name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w %q: extends may name built-in rulesets (%v) or ifc7 rulesets (https://%s/r/{owner}/{slug}/v{n})", ErrUnresolved, name, Builtins, RefHost)
		}
		if !ref.Pinned() {
			return nil, fmt.Errorf("%w: %s (add /v{n})", ErrUnpinned, name)
		}
		return load(ctx, ref)
	})
}

// canonicalName returns the name an extends entry is tracked under for
// cycle detection: the canonical URL for an ifc7 reference, so http, https
// and a trailing slash cannot disguise a cycle.
func canonicalName(name string) string {
	if ref, ok, err := ParseRef(name); ok && err == nil {
		return ref.String()
	}
	return name
}

// Refs returns the ifc7 references in doc's own extends list, in order.
// Malformed references are returned as an error.
func Refs(doc *Document) ([]Ref, error) {
	var out []Ref
	for _, e := range doc.Extends() {
		ref, ok, err := ParseRef(e.Name)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, ref)
		}
	}
	return out, nil
}
