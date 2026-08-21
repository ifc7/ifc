package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCreateMissingFileModel_enterOnYesConfirms(t *testing.T) {
	m := newCreateMissingFileModel("specs/api.yaml", "api")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*createMissingFileModel)
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if !m.confirmed || m.cancelled {
		t.Fatalf("expected confirmed, got confirmed=%v cancelled=%v", m.confirmed, m.cancelled)
	}
}

func TestCreateMissingFileModel_yConfirms(t *testing.T) {
	m := newCreateMissingFileModel("specs/api.yaml", "api")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = next.(*createMissingFileModel)
	if !m.confirmed || m.cancelled {
		t.Fatalf("expected confirmed, got confirmed=%v cancelled=%v", m.confirmed, m.cancelled)
	}
}

func TestCreateMissingFileModel_enterOnNoCancels(t *testing.T) {
	m := newCreateMissingFileModel("specs/api.yaml", "api")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*createMissingFileModel)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1 after down, got %d", m.cursor)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*createMissingFileModel)
	if m.confirmed || !m.cancelled {
		t.Fatalf("expected cancelled, got confirmed=%v cancelled=%v", m.confirmed, m.cancelled)
	}
}

func TestCreateMissingFileModel_nCancels(t *testing.T) {
	m := newCreateMissingFileModel("specs/api.yaml", "api")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(*createMissingFileModel)
	if m.confirmed || !m.cancelled {
		t.Fatalf("expected cancelled, got confirmed=%v cancelled=%v", m.confirmed, m.cancelled)
	}
}

func TestCreateMissingFileModel_viewMentionsPath(t *testing.T) {
	m := newCreateMissingFileModel("nested/dir/schema.json", "schema")
	view := m.View()
	if !strings.Contains(view, "nested/dir/schema.json") {
		t.Fatalf("expected view to mention path, got:\n%s", view)
	}
	if !strings.Contains(view, "schema") {
		t.Fatalf("expected view to mention name, got:\n%s", view)
	}
}
