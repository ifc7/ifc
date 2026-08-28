package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ifc7/ifc/internal/ui"
)

// PromptCreateMissingFile asks whether to create a specification file that does
// not yet exist at path. Returns nil if the user confirms, or an error if they
// cancel or the TUI fails.
func PromptCreateMissingFile(ctx context.Context, path, name string) error {
	_ = ctx
	model := newCreateMissingFileModel(path, name)
	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return fmt.Errorf("failed to start TUI: %w", err)
	}
	result, ok := finalModel.(*createMissingFileModel)
	if !ok {
		return fmt.Errorf("unexpected TUI result type")
	}
	if result.cancelled || !result.confirmed {
		return fmt.Errorf("cancelled")
	}
	return nil
}

type createMissingFileModel struct {
	path      string
	name      string
	cursor    int // 0 = yes, 1 = no
	confirmed bool
	cancelled bool
}

func newCreateMissingFileModel(path, name string) *createMissingFileModel {
	return &createMissingFileModel{
		path: path,
		name: name,
	}
}

func (m *createMissingFileModel) Init() tea.Cmd {
	return nil
}

func (m *createMissingFileModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "n", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "y":
			m.confirmed = true
			return m, tea.Quit
		case "up", "k":
			m.cursor = 0
		case "down", "j":
			m.cursor = 1
		case "enter":
			if m.cursor == 0 {
				m.confirmed = true
			} else {
				m.cancelled = true
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *createMissingFileModel) View() string {
	var builder strings.Builder
	fmt.Fprint(&builder, ui.ScreenTitle("Create file"))
	fmt.Fprintln(&builder, ui.Field("Name", m.name))
	fmt.Fprintln(&builder, ui.Field("Path", m.path))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, ui.Section("No file exists at this path. Create it (and any missing folders)?"))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, ui.ListRow(m.cursor == 0, m.cursor == 0, "Yes, create file"))
	fmt.Fprintln(&builder, ui.ListRow(m.cursor == 1, m.cursor == 1, "No, cancel"))
	fmt.Fprint(&builder, ui.KeyHints("\n↑/↓ move  ·  y/enter create  ·  n/q cancel"))
	return builder.String()
}
