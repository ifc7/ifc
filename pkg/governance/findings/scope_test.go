package findings

import (
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

func TestPointerContains(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		outer, inner string
		want         bool
	}{
		{"", "/paths", true},
		{"/paths/~1pets", "/paths/~1pets", true},
		{"/paths/~1pets", "/paths/~1pets/get/responses", true},
		{"/paths/~1pets", "/paths/~1petsx", false},
		{"/paths/~1pets/get", "/paths/~1pets", false},
	} {
		if got := PointerContains(tt.outer, tt.inner); got != tt.want {
			t.Errorf("PointerContains(%q, %q) = %v, want %v", tt.outer, tt.inner, got, tt.want)
		}
	}
}

func TestScopeToChanges(t *testing.T) {
	t.Parallel()
	fs := []contract.Finding{
		{RuleID: "scoped", Pointer: "/paths/~1pets/get"},
		{RuleID: "scoped", Pointer: "/paths/~1owners/get"},
		{RuleID: "other", Pointer: "/paths/~1owners/get"},
		{RuleID: "scoped", Pointer: "/paths/~1toys"},
	}
	changes := []contract.Change{
		{Kind: "pathItem.operation.added", Pointer: "/paths/~1pets/get"},
		{Kind: "pathItem.operation.modified", Pointer: "/paths/~1toys/get/description"},
	}
	t.Run("keeps scoped findings inside or around a change", func(t *testing.T) {
		t.Parallel()
		kept, dropped := ScopeToChanges(fs, []string{"scoped"}, changes, true)
		if len(kept) != 3 || kept[0].Pointer != "/paths/~1pets/get" || kept[1].RuleID != "other" || kept[2].Pointer != "/paths/~1toys" {
			t.Fatalf("kept = %+v", kept)
		}
		if len(dropped) != 1 || dropped[0].Pointer != "/paths/~1owners/get" {
			t.Fatalf("dropped = %+v", dropped)
		}
	})
	t.Run("keeps everything without a comparison", func(t *testing.T) {
		t.Parallel()
		kept, dropped := ScopeToChanges(fs, []string{"scoped"}, nil, false)
		if len(kept) != 4 || dropped != nil {
			t.Fatalf("kept %d dropped %d", len(kept), len(dropped))
		}
	})
	t.Run("drops every scoped finding when nothing changed", func(t *testing.T) {
		t.Parallel()
		kept, dropped := ScopeToChanges(fs, []string{"scoped"}, nil, true)
		if len(kept) != 1 || kept[0].RuleID != "other" || len(dropped) != 3 {
			t.Fatalf("kept %+v dropped %d", kept, len(dropped))
		}
	})
	t.Run("no scoped rules", func(t *testing.T) {
		t.Parallel()
		kept, _ := ScopeToChanges(fs, nil, changes, true)
		if len(kept) != 4 {
			t.Fatalf("kept %d", len(kept))
		}
	})
}
