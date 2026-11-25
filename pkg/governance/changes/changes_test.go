package changes_test

import (
	"strings"
	"testing"

	"github.com/ifc7/ifc/pkg/governance/changes"
	"github.com/ifc7/ifc/pkg/plugins/contract"
)

func TestNew(t *testing.T) {
	t.Parallel()
	b := changes.New("paths.path.removed", "/paths/~1a", true, "removed path")
	if b.Level != contract.ChangeLevelError || b.Fingerprint != changes.Fingerprint("paths.path.removed", "/paths/~1a") || len(b.Fingerprint) != 64 {
		t.Fatalf("breaking = %+v", b)
	}
	if n := changes.New("info.version.modified", "/info/version", false, "m"); n.Level != contract.ChangeLevelWarn {
		t.Fatalf("non-breaking = %+v", n)
	}
	if changes.Fingerprint("a", "bc") == changes.Fingerprint("ab", "c") {
		t.Fatal("fingerprint fields must be separated")
	}
}

func TestSort(t *testing.T) {
	t.Parallel()
	cs := []contract.Change{
		{Kind: "k.p.added", Pointer: "/a"},
		{Kind: "k.p.removed", Pointer: "/b", Breaking: true},
		{Kind: "k.p.removed", Pointer: "/a", Breaking: true},
	}
	changes.Sort(cs)
	if cs[0].Pointer != "/a" || !cs[0].Breaking || cs[1].Pointer != "/b" || cs[2].Breaking {
		t.Fatalf("sorted = %+v", cs)
	}
}

func TestCounts(t *testing.T) {
	t.Parallel()
	b, n := changes.Counts([]contract.Change{{Breaking: true}, {}, {}})
	if b != 1 || n != 2 {
		t.Fatalf("counts = %d, %d", b, n)
	}
}

func TestRenderText(t *testing.T) {
	t.Parallel()
	out := changes.RenderText([]contract.Change{
		{Kind: "paths.path.removed", Pointer: "/paths/~1a", Breaking: true, Message: "removed path"},
		{Kind: "pathItem.operation.removed", Pointer: "", Message: "removed get", Stability: "x-stability-level: draft"},
	})
	for _, want := range []string{"BREAKING paths.path.removed /paths/~1a: removed path\n", "NON-BREAKING pathItem.operation.removed /: removed get (exempt: x-stability-level: draft)\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestKind(t *testing.T) {
	t.Parallel()
	if got := changes.Kind("schema", "type", changes.ActionModified); got != "schema.type.modified" {
		t.Fatal(got)
	}
}
