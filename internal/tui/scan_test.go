package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ifc7/ifc/internal/client"
)

func TestScanAddModel_upDownNavigation(t *testing.T) {
	m := newScanAddModel([]ScanCandidate{
		{Path: "a.yaml", Type: client.OPENAPI, DefaultName: "a"},
		{Path: "b.yaml", Type: client.OPENAPI, DefaultName: "b"},
		{Path: "c.yaml", Type: client.JSONSCHEMA, DefaultName: "c"},
	})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*scanAddModel)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1 after down, got %d", m.cursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*scanAddModel)
	if m.cursor != 2 {
		t.Fatalf("expected cursor 2 after down, got %d", m.cursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(*scanAddModel)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1 after up, got %d", m.cursor)
	}

	// j/k should also move the selection cursor.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(*scanAddModel)
	if m.cursor != 2 {
		t.Fatalf("expected cursor 2 after j, got %d", m.cursor)
	}

	// Unknown CSI ending in A/B (some terminals) should also navigate.
	type unknownCSI []byte
	next, _ = m.Update(unknownCSI("\x1b[A"))
	m = next.(*scanAddModel)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1 after CSI up, got %d", m.cursor)
	}

	if !strings.Contains(m.View(), "b.yaml") {
		t.Fatalf("expected view to mention b.yaml")
	}
}

func TestNavDelta(t *testing.T) {
	if navDelta(tea.KeyMsg{Type: tea.KeyUp}) != -1 {
		t.Fatal("KeyUp")
	}
	if navDelta(tea.KeyMsg{Type: tea.KeyShiftDown}) != 1 {
		t.Fatal("KeyShiftDown")
	}
	if navDelta(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) != 1 {
		t.Fatal("j")
	}
	type unknownCSI []byte
	if navDelta(unknownCSI("\x1b[1;5A")) != -1 {
		t.Fatal("csi A")
	}
}

func TestScanAddModel_nameStepUpDown(t *testing.T) {
	m := newScanAddModel([]ScanCandidate{
		{Path: "a.yaml", Type: client.OPENAPI, DefaultName: "a"},
		{Path: "b.yaml", Type: client.OPENAPI, DefaultName: "b"},
	})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*scanAddModel)
	if m.step != scanStepName {
		t.Fatalf("expected name step, got %d", m.step)
	}
	if m.nameIndex != 0 {
		t.Fatalf("expected nameIndex 0, got %d", m.nameIndex)
	}

	m.nameInput.SetValue("alpha")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*scanAddModel)
	if m.nameIndex != 1 {
		t.Fatalf("expected nameIndex 1 after down, got %d", m.nameIndex)
	}
	if m.items[0].name != "alpha" {
		t.Fatalf("expected first name saved as alpha, got %q", m.items[0].name)
	}
}

func TestScanAddModel_canTypeQInName(t *testing.T) {
	m := newScanAddModel([]ScanCandidate{
		{Path: "a.yaml", Type: client.OPENAPI, DefaultName: "a"},
	})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*scanAddModel)
	_ = cmd

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(*scanAddModel)
	if m.cancelled {
		t.Fatal("typing q during name step should not cancel")
	}
}

func TestScanAddModel_scrollsLongList(t *testing.T) {
	candidates := make([]ScanCandidate, 40)
	for i := range candidates {
		candidates[i] = ScanCandidate{
			Path:        fmt.Sprintf("spec-%02d.yaml", i),
			Type:        client.OPENAPI,
			DefaultName: fmt.Sprintf("spec-%02d", i),
		}
	}
	m := newScanAddModel(candidates)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	m = next.(*scanAddModel)

	visible := m.visibleListRows()
	if visible >= 40 {
		t.Fatalf("expected window to clip list, visible=%d", visible)
	}

	// Move near the bottom; view should scroll and omit early items.
	for i := 0; i < 30; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(*scanAddModel)
	}
	view := m.View()
	if strings.Contains(view, "spec-00.yaml") {
		t.Fatalf("expected scrolled view to hide early items, got:\n%s", view)
	}
	if !strings.Contains(view, "spec-30.yaml") {
		t.Fatalf("expected scrolled view to show cursor item, got:\n%s", view)
	}
	if !strings.Contains(view, "more above") {
		t.Fatalf("expected 'more above' cue, got:\n%s", view)
	}

	// Page up should jump by a page.
	before := m.cursor
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = next.(*scanAddModel)
	if m.cursor >= before {
		t.Fatalf("expected page up to move cursor up, before=%d after=%d", before, m.cursor)
	}
}
