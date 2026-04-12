package tui

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/fileio"
)

type NewInterface struct {
	Name          string
	Description   string
	RevisionNotes string
	Type          client.InterfaceType
}

func PromptNewInterfaceCommit(ctx context.Context, name string) (NewInterface, error) {
	return NewInterface{
		Name:          name,
		Description:   "TODO",
		RevisionNotes: "TODO",
		Type:          client.OPENAPI,
	}, nil
}

type NewRevision struct {
	Notes string
}

func PromptNewRevisionCommit(ctx context.Context) (NewRevision, error) {
	return NewRevision{
		Notes: "TODO",
	}, nil
}

type InterfaceChange struct {
	InterfaceId client.InterfaceId
	Name        string
	Definition  []byte
}

type InterfaceRevisionUpdate struct {
	InterfaceId client.InterfaceId
	Definition  []byte
	Notes       string
}

func RunBubbleTeaPushChangesTui(changes []InterfaceChange) ([]InterfaceRevisionUpdate, bool, error) {
	model := newPushChangesModel(changes)
	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return nil, false, fmt.Errorf("failed to start TUI: %w", err)
	}
	result, ok := finalModel.(*pushChangesModel)
	if !ok {
		return nil, false, fmt.Errorf("unexpected TUI result type")
	}
	if result.cancelled {
		return nil, true, nil
	}
	return result.getSelectedUpdates(), false, nil
}

// pushChangesStep represents the steps in the push changes TUI flow.
type pushChangesStep int

const (
	pushStepSelectChanges pushChangesStep = iota
	pushStepAddNotes
	pushStepConfirm
)

// pushChangeItem holds a change with its selection state and notes.
type pushChangeItem struct {
	change   InterfaceChange
	selected bool
	notes    string
}

type pushChangesModel struct {
	step       pushChangesStep
	items      []pushChangeItem
	cursor     int
	notesInput textinput.Model
	notesIndex int // which selected item we're adding notes for
	errMsg     string
	cancelled  bool
}

func newPushChangesModel(changes []InterfaceChange) *pushChangesModel {
	items := make([]pushChangeItem, len(changes))
	for i, c := range changes {
		items[i] = pushChangeItem{change: c, selected: true}
	}
	notesInput := textinput.New()
	notesInput.Placeholder = "Optional revision notes"
	notesInput.Prompt = "> "
	notesInput.CharLimit = 280
	notesInput.Width = 60
	return &pushChangesModel{
		step:       pushStepSelectChanges,
		items:      items,
		cursor:     0,
		notesInput: notesInput,
		notesIndex: 0,
	}
}

func (m *pushChangesModel) getSelectedUpdates() []InterfaceRevisionUpdate {
	var updates []InterfaceRevisionUpdate
	for _, item := range m.items {
		if item.selected {
			updates = append(updates, InterfaceRevisionUpdate{
				InterfaceId: item.change.InterfaceId,
				Definition:  item.change.Definition,
				Notes:       strings.TrimSpace(item.notes),
			})
		}
	}
	return updates
}

func (m *pushChangesModel) getSelectedItems() []*pushChangeItem {
	var selected []*pushChangeItem
	for i := range m.items {
		if m.items[i].selected {
			selected = append(selected, &m.items[i])
		}
	}
	return selected
}

func (m *pushChangesModel) Init() tea.Cmd {
	return nil
}

func (m *pushChangesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.cancelled = true
			return m, tea.Quit
		case "esc":
			if m.step > pushStepSelectChanges {
				m.step--
				m.errMsg = ""
				m.applyFocus()
				return m, nil
			}
		}
	}

	switch m.step {
	case pushStepSelectChanges:
		return m.updateSelectChanges(msg)
	case pushStepAddNotes:
		return m.updateAddNotes(msg)
	case pushStepConfirm:
		return m.updateConfirm(msg)
	default:
		return m, nil
	}
}

func (m *pushChangesModel) updateSelectChanges(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ":
			m.items[m.cursor].selected = !m.items[m.cursor].selected
		case "enter":
			m.errMsg = ""
			if m.countSelected() == 0 {
				m.errMsg = "Select at least one change to push."
				return m, nil
			}
			m.step = pushStepAddNotes
			m.notesIndex = 0
			selected := m.getSelectedItems()
			if len(selected) > 0 {
				m.notesInput.SetValue(selected[0].notes)
			}
			m.applyFocus()
		}
	}
	return m, nil
}

func (m *pushChangesModel) countSelected() int {
	n := 0
	for _, item := range m.items {
		if item.selected {
			n++
		}
	}
	return n
}

func (m *pushChangesModel) updateAddNotes(msg tea.Msg) (tea.Model, tea.Cmd) {
	selected := m.getSelectedItems()
	if len(selected) == 0 {
		m.step = pushStepConfirm
		return m, nil
	}
	current := selected[m.notesIndex]

	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		current.notes = strings.TrimSpace(m.notesInput.Value())
		m.notesInput.SetValue("")
		if m.notesIndex < len(selected)-1 {
			m.notesIndex++
			m.notesInput.SetValue(selected[m.notesIndex].notes)
			m.applyFocus()
		} else {
			m.step = pushStepConfirm
			m.applyFocus()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.notesInput, cmd = m.notesInput.Update(msg)
	return m, cmd
}

func (m *pushChangesModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "backspace":
			selected := m.getSelectedItems()
			if len(selected) > 0 {
				m.step = pushStepAddNotes
				m.notesIndex = len(selected) - 1
				m.notesInput.SetValue(selected[m.notesIndex].notes)
				m.applyFocus()
			}
		}
	}
	return m, nil
}

func (m *pushChangesModel) applyFocus() {
	if m.step == pushStepAddNotes {
		m.notesInput.Focus()
	} else {
		m.notesInput.Blur()
	}
}

func (m *pushChangesModel) View() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Push changes to server\n\n")

	switch m.step {
	case pushStepSelectChanges:
		fmt.Fprintf(&builder, "Select changes to push (Space to toggle):\n\n")
		for i, item := range m.items {
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			check := " "
			if item.selected {
				check = "x"
			}
			fmt.Fprintf(&builder, "%s [%s] %s\n", cursor, check, item.change.Name)
		}
		fmt.Fprintf(&builder, "\nUse ↑/↓ to move, Space to toggle, Enter to continue.")
	case pushStepAddNotes:
		selected := m.getSelectedItems()
		if len(selected) == 0 {
			fmt.Fprintf(&builder, "No changes selected.\n")
		} else {
			current := selected[m.notesIndex]
			fmt.Fprintf(&builder, "Notes for %s (%d of %d):\n\n", current.change.Name, m.notesIndex+1, len(selected))
			fmt.Fprint(&builder, m.notesInput.View())
			fmt.Fprintf(&builder, "\n\nPress Enter to continue.")
		}
	case pushStepConfirm:
		fmt.Fprintf(&builder, "Confirm push:\n\n")
		for _, item := range m.items {
			if !item.selected {
				continue
			}
			notes := item.notes
			if notes == "" {
				notes = "(none)"
			}
			fmt.Fprintf(&builder, "  %s - notes: %s\n", item.change.Name, notes)
		}
		fmt.Fprintf(&builder, "\nPress Enter to submit or Backspace to edit notes.")
	}

	if m.errMsg != "" {
		fmt.Fprintf(&builder, "\n\n")
		fmt.Fprint(&builder, m.errMsg)
	}

	fmt.Fprintf(&builder, "\n\nPress q to quit.\n")
	return builder.String()
}

func createNewInterface(ctx context.Context, cl *client.ClientWithResponses, file string) error {
	fileBytes, err := fileio.ReadFile(file)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}
	fmt.Println("Interface does not exist on the server, creating new interface...")
	tuiResponse := runBubleTeaNewInterfaceTui()
	resp, err := cl.CreateInterfaceWithResponse(ctx, client.CreateInterfaceRequest{
		Definition:  base64.StdEncoding.EncodeToString(fileBytes),
		Description: tuiResponse.Description,
		Name:        tuiResponse.Name,
		Type:        tuiResponse.Type,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode(), string(resp.Body))
	}
	return nil
}

type newInterfaceTuiResponse struct {
	Name        string
	Description *string
	Type        client.InterfaceType
}

func runBubleTeaNewInterfaceTui() newInterfaceTuiResponse {
	model := newNewInterfaceModel()
	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		fmt.Println("Failed to start TUI:", err)
		os.Exit(1)
	}
	result, ok := finalModel.(*newInterfaceModel)
	if !ok {
		fmt.Println("Unexpected TUI result type")
		os.Exit(1)
	}
	if result.cancelled {
		fmt.Println("Cancelled.")
		os.Exit(1)
	}
	name := strings.TrimSpace(result.nameInput.Value())
	description := strings.TrimSpace(result.descriptionInput.Value())
	var descriptionPtr *string
	if description != "" {
		descriptionPtr = &description
	}
	return newInterfaceTuiResponse{
		Name:        name,
		Description: descriptionPtr,
		Type:        result.typeOptions[result.typeIndex].Value,
	}
}

type newInterfaceStep int

const (
	stepSelectType newInterfaceStep = iota
	stepName
	stepDescription
	stepConfirm
)

type interfaceTypeOption struct {
	Value client.InterfaceType
	Label string
	Help  string
}

type newInterfaceModel struct {
	step             newInterfaceStep
	typeOptions      []interfaceTypeOption
	typeIndex        int
	nameInput        textinput.Model
	descriptionInput textinput.Model
	errMsg           string
	cancelled        bool
}

func newNewInterfaceModel() *newInterfaceModel {
	nameInput := textinput.New()
	nameInput.Placeholder = "e.g. Billing API"
	nameInput.Prompt = "> "
	nameInput.CharLimit = 120
	nameInput.Width = 60

	descriptionInput := textinput.New()
	descriptionInput.Placeholder = "Optional description"
	descriptionInput.Prompt = "> "
	descriptionInput.CharLimit = 280
	descriptionInput.Width = 60

	return &newInterfaceModel{
		step: stepSelectType,
		typeOptions: []interfaceTypeOption{
			{
				Value: client.OPENAPI,
				Label: "OPENAPI",
				Help:  "OpenAPI specification",
			},
			{
				Value: client.JSONSCHEMA,
				Label: "JSON_SCHEMA",
				Help:  "JSON Schema definition",
			},
		},
		typeIndex:        0,
		nameInput:        nameInput,
		descriptionInput: descriptionInput,
	}
}

func (m *newInterfaceModel) Init() tea.Cmd {
	return nil
}

func (m *newInterfaceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.cancelled = true
			return m, tea.Quit
		case "esc":
			if m.step > stepSelectType {
				m.step--
				m.errMsg = ""
				m.applyFocus()
				return m, nil
			}
		}
	}

	switch m.step {
	case stepSelectType:
		return m.updateTypeSelection(msg)
	case stepName:
		return m.updateName(msg)
	case stepDescription:
		return m.updateDescription(msg)
	case stepConfirm:
		return m.updateConfirm(msg)
	default:
		return m, nil
	}
}

func (m *newInterfaceModel) updateTypeSelection(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.typeIndex > 0 {
				m.typeIndex--
			}
		case "down", "j":
			if m.typeIndex < len(m.typeOptions)-1 {
				m.typeIndex++
			}
		case "enter":
			m.step = stepName
			m.errMsg = ""
			m.applyFocus()
		}
	}
	return m, nil
}

func (m *newInterfaceModel) updateName(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		if strings.TrimSpace(m.nameInput.Value()) == "" {
			m.errMsg = "Name is required."
			return m, nil
		}
		m.step = stepDescription
		m.errMsg = ""
		m.applyFocus()
		return m, nil
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m *newInterfaceModel) updateDescription(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.step = stepConfirm
		m.errMsg = ""
		m.applyFocus()
		return m, nil
	}

	var cmd tea.Cmd
	m.descriptionInput, cmd = m.descriptionInput.Update(msg)
	return m, cmd
}

func (m *newInterfaceModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "backspace":
			m.step = stepDescription
			m.applyFocus()
		}
	}
	return m, nil
}

func (m *newInterfaceModel) applyFocus() {
	switch m.step {
	case stepName:
		m.nameInput.Focus()
		m.descriptionInput.Blur()
	case stepDescription:
		m.nameInput.Blur()
		m.descriptionInput.Focus()
	default:
		m.nameInput.Blur()
		m.descriptionInput.Blur()
	}
}

func (m *newInterfaceModel) View() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Create a new interface\n\n")

	switch m.step {
	case stepSelectType:
		fmt.Fprintf(&builder, "Select interface type:\n\n")
		for i, option := range m.typeOptions {
			cursor := " "
			if i == m.typeIndex {
				cursor = ">"
			}
			check := " "
			if i == m.typeIndex {
				check = "x"
			}
			fmt.Fprintf(&builder, "%s [%s] %s - %s\n", cursor, check, option.Label, option.Help)
		}
		fmt.Fprintf(&builder, "\nUse ↑/↓ and press Enter to continue.")
	case stepName:
		fmt.Fprintf(&builder, "Interface name (required):\n\n")
		fmt.Fprint(&builder, m.nameInput.View())
		fmt.Fprintf(&builder, "\n\nPress Enter to continue.")
	case stepDescription:
		fmt.Fprintf(&builder, "Description (optional):\n\n")
		fmt.Fprint(&builder, m.descriptionInput.View())
		fmt.Fprintf(&builder, "\n\nPress Enter to continue.")
	case stepConfirm:
		fmt.Fprintf(&builder, "Confirm details:\n\n")
		fmt.Fprintf(&builder, "Type: %s\n", m.typeOptions[m.typeIndex].Label)
		fmt.Fprintf(&builder, "Name: %s\n", strings.TrimSpace(m.nameInput.Value()))
		description := strings.TrimSpace(m.descriptionInput.Value())
		if description == "" {
			description = "(none)"
		}
		fmt.Fprintf(&builder, "Description: %s\n", description)
		fmt.Fprintf(&builder, "\nPress Enter to submit or Backspace to edit.")
	}

	if m.errMsg != "" {
		fmt.Fprintf(&builder, "\n\n")
		fmt.Fprint(&builder, m.errMsg)
	}

	fmt.Fprintf(&builder, "\n\nPress q to quit.")
	return builder.String()
}
