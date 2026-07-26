package tui

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/fileio"
)

type NewInterface struct {
	Name          string
	Description   string
	RevisionNotes string
	Type          client.InterfaceType
}

func PromptNewInterfaceCommit(ctx context.Context, name string, ifaceType client.InterfaceType) (NewInterface, error) {
	_ = ctx
	model := newNewInterfaceCommitModel(name, ifaceType)
	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return NewInterface{}, fmt.Errorf("failed to start TUI: %w", err)
	}
	result, ok := finalModel.(*newInterfaceCommitModel)
	if !ok {
		return NewInterface{}, fmt.Errorf("unexpected TUI result type")
	}
	if result.cancelled {
		return NewInterface{}, fmt.Errorf("cancelled")
	}
	return NewInterface{
		Name:          name,
		Description:   strings.TrimSpace(result.descriptionInput.Value()),
		RevisionNotes: strings.TrimSpace(result.notesInput.Value()),
		Type:          result.ifaceType,
	}, nil
}

type NewRevision struct {
	Notes string
}

func PromptNewRevisionCommit(ctx context.Context, name string) (NewRevision, error) {
	_ = ctx
	model := newNewRevisionCommitModel(name)
	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return NewRevision{}, fmt.Errorf("failed to start TUI: %w", err)
	}
	result, ok := finalModel.(*newRevisionCommitModel)
	if !ok {
		return NewRevision{}, fmt.Errorf("unexpected TUI result type")
	}
	if result.cancelled {
		return NewRevision{}, fmt.Errorf("cancelled")
	}
	return NewRevision{
		Notes: strings.TrimSpace(result.notesInput.Value()),
	}, nil
}

type InterfaceOwnerOption struct {
	ID    string // user or org ID for CreateInterfaceRequest.Owner
	Label string // shown in the list
	Kind  string // "user" or "org" for display help
}

func PromptInterfaceOwner(ctx context.Context, interfaceName string, options []InterfaceOwnerOption) (InterfaceOwnerOption, error) {
	_ = ctx
	if len(options) == 0 {
		return InterfaceOwnerOption{}, fmt.Errorf("no owner options available")
	}
	model := newInterfaceOwnerModel(interfaceName, options)
	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return InterfaceOwnerOption{}, fmt.Errorf("failed to start TUI: %w", err)
	}
	result, ok := finalModel.(*interfaceOwnerModel)
	if !ok {
		return InterfaceOwnerOption{}, fmt.Errorf("unexpected TUI result type")
	}
	if result.cancelled {
		return InterfaceOwnerOption{}, fmt.Errorf("cancelled")
	}
	return result.options[result.cursor], nil
}

type interfaceOwnerStep int

const (
	ownerStepSelect interfaceOwnerStep = iota
	ownerStepConfirm
)

type interfaceOwnerModel struct {
	interfaceName string
	options       []InterfaceOwnerOption
	cursor        int
	step          interfaceOwnerStep
	cancelled     bool
}

func newInterfaceOwnerModel(interfaceName string, options []InterfaceOwnerOption) *interfaceOwnerModel {
	return &interfaceOwnerModel{
		interfaceName: interfaceName,
		options:       options,
		cursor:        0,
		step:          ownerStepSelect,
	}
}

func (m *interfaceOwnerModel) Init() tea.Cmd {
	return nil
}

func (m *interfaceOwnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "q":
			if m.step == ownerStepConfirm {
				m.cancelled = true
				return m, tea.Quit
			}
		case "esc":
			if m.step > ownerStepSelect {
				m.step--
				return m, nil
			}
		}
	}

	switch m.step {
	case ownerStepSelect:
		return m.updateSelect(msg)
	case ownerStepConfirm:
		return m.updateConfirm(msg)
	default:
		return m, nil
	}
}

func (m *interfaceOwnerModel) updateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter":
			m.step = ownerStepConfirm
		case "q":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *interfaceOwnerModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "backspace":
			m.step = ownerStepSelect
		}
	}
	return m, nil
}

func (m *interfaceOwnerModel) View() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Choose owner for interface: %s\n\n", m.interfaceName)

	switch m.step {
	case ownerStepSelect:
		fmt.Fprintf(&builder, "Select owner:\n\n")
		for i, option := range m.options {
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			check := " "
			if i == m.cursor {
				check = "x"
			}
			kind := option.Kind
			if kind == "" {
				kind = "owner"
			}
			fmt.Fprintf(&builder, "%s [%s] %s (%s)\n", cursor, check, option.Label, kind)
		}
		fmt.Fprintf(&builder, "\nUse ↑/↓ and press Enter to continue. Press q to cancel.")
	case ownerStepConfirm:
		selected := m.options[m.cursor]
		fmt.Fprintf(&builder, "Confirm owner:\n\n")
		fmt.Fprintf(&builder, "Interface: %s\n", m.interfaceName)
		fmt.Fprintf(&builder, "Owner: %s (%s)\n", selected.Label, selected.Kind)
		fmt.Fprintf(&builder, "\nPress Enter to submit, Backspace to edit, or q to cancel.")
	}

	return builder.String()
}

type newInterfaceCommitStep int

const (
	newIfcCommitStepDescription newInterfaceCommitStep = iota
	newIfcCommitStepNotes
	newIfcCommitStepConfirm
)

type newInterfaceCommitModel struct {
	name             string
	ifaceType        client.InterfaceType
	step             newInterfaceCommitStep
	descriptionInput textinput.Model
	notesInput       textinput.Model
	cancelled        bool
}

func newNewInterfaceCommitModel(name string, ifaceType client.InterfaceType) *newInterfaceCommitModel {
	descriptionInput := textinput.New()
	descriptionInput.Placeholder = "Optional description"
	descriptionInput.Prompt = "> "
	descriptionInput.CharLimit = 280
	descriptionInput.Width = 60
	descriptionInput.Focus()

	notesInput := textinput.New()
	notesInput.Placeholder = "Optional revision notes"
	notesInput.Prompt = "> "
	notesInput.CharLimit = 280
	notesInput.Width = 60

	return &newInterfaceCommitModel{
		name:             name,
		ifaceType:        ifaceType,
		step:             newIfcCommitStepDescription,
		descriptionInput: descriptionInput,
		notesInput:       notesInput,
	}
}

func (m *newInterfaceCommitModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *newInterfaceCommitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "q":
			// Only cancel on confirm so "q" can be typed in text fields.
			if m.step == newIfcCommitStepConfirm {
				m.cancelled = true
				return m, tea.Quit
			}
		case "esc":
			if m.step > newIfcCommitStepDescription {
				m.step--
				m.applyFocus()
				return m, nil
			}
		}
	}

	switch m.step {
	case newIfcCommitStepDescription:
		return m.updateDescription(msg)
	case newIfcCommitStepNotes:
		return m.updateNotes(msg)
	case newIfcCommitStepConfirm:
		return m.updateConfirm(msg)
	default:
		return m, nil
	}
}

func (m *newInterfaceCommitModel) updateDescription(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.step = newIfcCommitStepNotes
		m.applyFocus()
		return m, nil
	}
	var cmd tea.Cmd
	m.descriptionInput, cmd = m.descriptionInput.Update(msg)
	return m, cmd
}

func (m *newInterfaceCommitModel) updateNotes(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.step = newIfcCommitStepConfirm
		m.applyFocus()
		return m, nil
	}
	var cmd tea.Cmd
	m.notesInput, cmd = m.notesInput.Update(msg)
	return m, cmd
}

func (m *newInterfaceCommitModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "backspace":
			m.step = newIfcCommitStepNotes
			m.applyFocus()
		}
	}
	return m, nil
}

func (m *newInterfaceCommitModel) applyFocus() {
	switch m.step {
	case newIfcCommitStepDescription:
		m.descriptionInput.Focus()
		m.notesInput.Blur()
	case newIfcCommitStepNotes:
		m.descriptionInput.Blur()
		m.notesInput.Focus()
	default:
		m.descriptionInput.Blur()
		m.notesInput.Blur()
	}
}

func (m *newInterfaceCommitModel) View() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Commit new interface: %s (%s)\n\n", m.name, m.ifaceType)

	switch m.step {
	case newIfcCommitStepDescription:
		fmt.Fprintf(&builder, "Description (optional):\n\n")
		fmt.Fprint(&builder, m.descriptionInput.View())
		fmt.Fprintf(&builder, "\n\nPress Enter to continue.")
	case newIfcCommitStepNotes:
		fmt.Fprintf(&builder, "Initial revision notes (optional):\n\n")
		fmt.Fprint(&builder, m.notesInput.View())
		fmt.Fprintf(&builder, "\n\nPress Enter to continue.")
	case newIfcCommitStepConfirm:
		description := strings.TrimSpace(m.descriptionInput.Value())
		if description == "" {
			description = "(none)"
		}
		notes := strings.TrimSpace(m.notesInput.Value())
		if notes == "" {
			notes = "(none)"
		}
		fmt.Fprintf(&builder, "Confirm details:\n\n")
		fmt.Fprintf(&builder, "Name: %s\n", m.name)
		fmt.Fprintf(&builder, "Type: %s\n", m.ifaceType)
		fmt.Fprintf(&builder, "Description: %s\n", description)
		fmt.Fprintf(&builder, "Revision notes: %s\n", notes)
		fmt.Fprintf(&builder, "\nPress Enter to submit or Backspace to edit.")
	}

	return builder.String()
}

type newRevisionCommitStep int

const (
	newRevCommitStepNotes newRevisionCommitStep = iota
	newRevCommitStepConfirm
)

type newRevisionCommitModel struct {
	name       string
	step       newRevisionCommitStep
	notesInput textinput.Model
	cancelled  bool
}

func newNewRevisionCommitModel(name string) *newRevisionCommitModel {
	notesInput := textinput.New()
	notesInput.Placeholder = "Optional revision notes"
	notesInput.Prompt = "> "
	notesInput.CharLimit = 280
	notesInput.Width = 60
	notesInput.Focus()

	return &newRevisionCommitModel{
		name:       name,
		step:       newRevCommitStepNotes,
		notesInput: notesInput,
	}
}

func (m *newRevisionCommitModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *newRevisionCommitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "q":
			// Only cancel on confirm so "q" can be typed in text fields.
			if m.step == newRevCommitStepConfirm {
				m.cancelled = true
				return m, tea.Quit
			}
		case "esc":
			if m.step > newRevCommitStepNotes {
				m.step--
				m.applyFocus()
				return m, nil
			}
		}
	}

	switch m.step {
	case newRevCommitStepNotes:
		return m.updateNotes(msg)
	case newRevCommitStepConfirm:
		return m.updateConfirm(msg)
	default:
		return m, nil
	}
}

func (m *newRevisionCommitModel) updateNotes(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.step = newRevCommitStepConfirm
		m.applyFocus()
		return m, nil
	}
	var cmd tea.Cmd
	m.notesInput, cmd = m.notesInput.Update(msg)
	return m, cmd
}

func (m *newRevisionCommitModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "backspace":
			m.step = newRevCommitStepNotes
			m.applyFocus()
		}
	}
	return m, nil
}

func (m *newRevisionCommitModel) applyFocus() {
	if m.step == newRevCommitStepNotes {
		m.notesInput.Focus()
	} else {
		m.notesInput.Blur()
	}
}

func (m *newRevisionCommitModel) View() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Commit new revision: %s\n\n", m.name)

	switch m.step {
	case newRevCommitStepNotes:
		fmt.Fprintf(&builder, "Revision notes for %s (optional):\n\n", m.name)
		fmt.Fprint(&builder, m.notesInput.View())
		fmt.Fprintf(&builder, "\n\nPress Enter to continue.")
	case newRevCommitStepConfirm:
		notes := strings.TrimSpace(m.notesInput.Value())
		if notes == "" {
			notes = "(none)"
		}
		fmt.Fprintf(&builder, "Confirm details:\n\n")
		fmt.Fprintf(&builder, "Interface: %s\n", m.name)
		fmt.Fprintf(&builder, "Notes: %s\n", notes)
		fmt.Fprintf(&builder, "\nPress Enter to submit or Backspace to edit.")
	}

	return builder.String()
}

type InterfaceChange struct {
	InterfaceId   client.InterfaceId
	Name          string
	Specification []byte
}

type InterfaceRevisionUpdate struct {
	InterfaceId   client.InterfaceId
	Specification []byte
	Notes         string
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
				InterfaceId:   item.change.InterfaceId,
				Specification: item.change.Specification,
				Notes:         strings.TrimSpace(item.notes),
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
	userID, err := cl.CurrentUserID(ctx)
	if err != nil {
		return err
	}
	tuiResponse := runBubleTeaNewInterfaceTui()
	resp, err := cl.CreateInterfaceWithResponse(ctx, client.CreateInterfaceRequest{
		Description: tuiResponse.Description,
		Name:        tuiResponse.Name,
		Type:        tuiResponse.Type,
		Owner:       userID,
		IsPublic:    false, // TODO: gather public status from user
	})
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode(), string(resp.Body))
	}
	_, err = cl.CreateInterfaceRevisionWithResponse(ctx, resp.JSON201.Id, client.CreateRevisionRequest{
		Specification: base64.StdEncoding.EncodeToString(fileBytes),
		CreatedBy:     userID,
	})
	if err != nil {
		return err
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

// ScanCandidate is an untracked specification offered for addition.
type ScanCandidate struct {
	Path        string
	Type        client.InterfaceType
	DefaultName string
}

// ScanSelection is a user-chosen candidate with the name to use in ifc.yaml.
type ScanSelection struct {
	Path string
	Name string
	Type client.InterfaceType
}

// PromptScanAdd presents untracked interface specs for multi-select and naming.
func PromptScanAdd(ctx context.Context, candidates []ScanCandidate) ([]ScanSelection, error) {
	_ = ctx
	if len(candidates) == 0 {
		return nil, nil
	}
	model := newScanAddModel(candidates)
	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithInputTTY(),
		tea.WithoutBracketedPaste(),
	)
	finalModel, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to start TUI: %w", err)
	}
	result, ok := finalModel.(*scanAddModel)
	if !ok {
		return nil, fmt.Errorf("unexpected TUI result type")
	}
	if result.cancelled {
		return nil, fmt.Errorf("cancelled")
	}
	return result.selections(), nil
}

type scanAddStep int

const (
	scanStepSelect scanAddStep = iota
	scanStepName
	scanStepConfirm
)

type scanAddItem struct {
	candidate ScanCandidate
	selected  bool
	name      string
}

type scanAddModel struct {
	step      scanAddStep
	items     []scanAddItem
	cursor    int
	offset    int // first visible item index in the select list
	height    int // terminal height
	width     int // terminal width
	nameIndex int
	nameInput textinput.Model
	errMsg    string
	cancelled bool
	lastKey   string
	debugKeys bool
}

// Lines reserved for title, help, scroll cues, and footer around the list.
const scanListChromeLines = 10
const scanListMinVisible = 3
const scanListDefaultHeight = 24

func newScanAddModel(candidates []ScanCandidate) *scanAddModel {
	items := make([]scanAddItem, len(candidates))
	for i, c := range candidates {
		items[i] = scanAddItem{
			candidate: c,
			selected:  true,
			name:      c.DefaultName,
		}
	}
	nameInput := textinput.New()
	nameInput.Placeholder = "interface name"
	nameInput.Prompt = "> "
	nameInput.CharLimit = 120
	nameInput.Width = 60
	// Disable suggestion bindings so ↑/↓ are available for list navigation.
	nameInput.KeyMap.NextSuggestion = key.NewBinding()
	nameInput.KeyMap.PrevSuggestion = key.NewBinding()
	nameInput.Blur()
	m := &scanAddModel{
		step:      scanStepSelect,
		items:     items,
		cursor:    0,
		nameInput: nameInput,
		debugKeys: os.Getenv("IFC_DEBUG_KEYS") != "",
	}
	_ = m.applyFocus()
	return m
}

func (m *scanAddModel) selections() []ScanSelection {
	var out []ScanSelection
	for _, item := range m.items {
		if !item.selected {
			continue
		}
		out = append(out, ScanSelection{
			Path: item.candidate.Path,
			Name: item.name,
			Type: item.candidate.Type,
		})
	}
	return out
}

func (m *scanAddModel) selectedItems() []*scanAddItem {
	var out []*scanAddItem
	for i := range m.items {
		if m.items[i].selected {
			out = append(out, &m.items[i])
		}
	}
	return out
}

func (m *scanAddModel) Init() tea.Cmd {
	return nil
}

func (m *scanAddModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.noteKey(msg)

	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = size.Height
		m.width = size.Width
		m.ensureCursorVisible()
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case keyMsg.String() == "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case keyMsg.String() == "q" && m.step != scanStepName:
			// Allow typing "q" while naming; quit elsewhere.
			m.cancelled = true
			return m, tea.Quit
		case keyMsg.String() == "esc" && m.step > scanStepSelect:
			if m.step == scanStepName {
				m.saveCurrentName()
			}
			m.step--
			m.errMsg = ""
			return m, m.applyFocus()
		}
	}

	switch m.step {
	case scanStepSelect:
		return m.updateSelect(msg)
	case scanStepName:
		return m.updateName(msg)
	case scanStepConfirm:
		return m.updateConfirm(msg)
	default:
		return m, nil
	}
}

func (m *scanAddModel) updateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if page := pageDelta(msg); page != 0 {
		m.cursor += page * m.visibleListRows()
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor >= len(m.items) {
			m.cursor = len(m.items) - 1
		}
		m.ensureCursorVisible()
		return m, nil
	}

	if delta := navDelta(msg); delta != 0 {
		m.cursor += delta
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor >= len(m.items) {
			m.cursor = len(m.items) - 1
		}
		m.ensureCursorVisible()
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case keyMsg.String() == " " || keyMsg.Type == tea.KeySpace:
		m.items[m.cursor].selected = !m.items[m.cursor].selected
	case keyMsg.String() == "enter" || keyMsg.Type == tea.KeyEnter:
		m.errMsg = ""
		selected := m.selectedItems()
		if len(selected) == 0 {
			return m, tea.Quit
		}
		m.step = scanStepName
		m.nameIndex = 0
		m.nameInput.SetValue(selected[0].name)
		return m, m.applyFocus()
	case keyMsg.String() == "home" || keyMsg.Type == tea.KeyHome:
		m.cursor = 0
		m.ensureCursorVisible()
	case keyMsg.String() == "end" || keyMsg.Type == tea.KeyEnd:
		if len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		}
		m.ensureCursorVisible()
	}
	return m, nil
}

func (m *scanAddModel) updateName(msg tea.Msg) (tea.Model, tea.Cmd) {
	selected := m.selectedItems()
	if len(selected) == 0 {
		m.step = scanStepConfirm
		return m, m.applyFocus()
	}

	// Arrow/tab navigation between names — keep j/k typable in the input.
	if delta := navDeltaArrowsOnly(msg); delta != 0 {
		m.saveCurrentName()
		m.nameIndex += delta
		if m.nameIndex < 0 {
			m.nameIndex = 0
		}
		if m.nameIndex >= len(selected) {
			m.nameIndex = len(selected) - 1
		}
		m.nameInput.SetValue(selected[m.nameIndex].name)
		return m, m.applyFocus()
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" || keyMsg.Type == tea.KeyEnter {
			name := strings.TrimSpace(m.nameInput.Value())
			if name == "" {
				m.errMsg = "Name is required."
				return m, nil
			}
			for i, item := range selected {
				if i != m.nameIndex && item.name == name {
					m.errMsg = "Name already used by another selection."
					return m, nil
				}
			}
			selected[m.nameIndex].name = name
			m.errMsg = ""
			if m.nameIndex < len(selected)-1 {
				m.nameIndex++
				m.nameInput.SetValue(selected[m.nameIndex].name)
				return m, m.applyFocus()
			}
			m.step = scanStepConfirm
			return m, m.applyFocus()
		}
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m *scanAddModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if delta := navDelta(msg); delta != 0 {
		selected := m.selectedItems()
		if len(selected) > 0 {
			m.step = scanStepName
			if delta < 0 {
				m.nameIndex = len(selected) - 1
			} else {
				m.nameIndex = 0
			}
			m.nameInput.SetValue(selected[m.nameIndex].name)
			return m, m.applyFocus()
		}
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case keyMsg.String() == "enter" || keyMsg.Type == tea.KeyEnter:
		return m, tea.Quit
	case keyMsg.String() == "backspace" || keyMsg.Type == tea.KeyBackspace:
		selected := m.selectedItems()
		if len(selected) > 0 {
			m.step = scanStepName
			m.nameIndex = len(selected) - 1
			m.nameInput.SetValue(selected[m.nameIndex].name)
			return m, m.applyFocus()
		}
	}
	return m, nil
}

func (m *scanAddModel) noteKey(msg tea.Msg) {
	if !m.debugKeys {
		return
	}
	switch keyMsg := msg.(type) {
	case tea.KeyMsg:
		m.lastKey = fmt.Sprintf("KeyMsg type=%d string=%q", int(keyMsg.Type), keyMsg.String())
	default:
		m.lastKey = fmt.Sprintf("%T %v", msg, msg)
	}
}

func (m *scanAddModel) saveCurrentName() {
	selected := m.selectedItems()
	if m.step != scanStepName || len(selected) == 0 || m.nameIndex >= len(selected) {
		return
	}
	if name := strings.TrimSpace(m.nameInput.Value()); name != "" {
		selected[m.nameIndex].name = name
	}
}

func (m *scanAddModel) applyFocus() tea.Cmd {
	if m.step == scanStepName {
		return m.nameInput.Focus()
	}
	m.nameInput.Blur()
	return nil
}

// navDelta returns -1 (up), 1 (down), or 0 for single-step movement.
func navDelta(msg tea.Msg) int {
	if d := navDeltaArrowsOnly(msg); d != 0 {
		return d
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return 0
	}
	switch keyMsg.String() {
	case "k", "ctrl+p":
		return -1
	case "j", "ctrl+n", "tab":
		return 1
	case "shift+tab":
		return -1
	}
	return 0
}

// pageDelta returns -1 (page up), 1 (page down), or 0.
func pageDelta(msg tea.Msg) int {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return 0
	}
	switch keyMsg.Type {
	case tea.KeyPgUp:
		return -1
	case tea.KeyPgDown:
		return 1
	}
	switch keyMsg.String() {
	case "pgup", "ctrl+u":
		return -1
	case "pgdown", "ctrl+d":
		return 1
	}
	return 0
}

func navDeltaArrowsOnly(msg tea.Msg) int {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyUp, tea.KeyShiftUp, tea.KeyCtrlUp, tea.KeyCtrlShiftUp:
			return -1
		case tea.KeyDown, tea.KeyShiftDown, tea.KeyCtrlDown, tea.KeyCtrlShiftDown:
			return 1
		}
		s := keyMsg.String()
		switch {
		case s == "up" || strings.HasSuffix(s, "+up"):
			return -1
		case s == "down" || strings.HasSuffix(s, "+down"):
			return 1
		}
	}
	// Some terminals (or keyboard protocols) deliver arrows as unrecognized
	// CSI sequences. bubbletea reports those as an unexported []byte message
	// whose last byte is A/B/C/D for the arrow direction.
	if b := byteSliceMsg(msg); len(b) > 0 {
		switch b[len(b)-1] {
		case 'A':
			return -1
		case 'B':
			return 1
		}
	}
	return 0
}

func byteSliceMsg(msg tea.Msg) []byte {
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Uint8 {
		return nil
	}
	return v.Bytes()
}

func (m *scanAddModel) visibleListRows() int {
	h := m.height
	if h <= 0 {
		h = scanListDefaultHeight
	}
	rows := h - scanListChromeLines
	if rows < scanListMinVisible {
		return scanListMinVisible
	}
	return rows
}

func (m *scanAddModel) ensureCursorVisible() {
	if len(m.items) == 0 {
		m.offset = 0
		return
	}
	visible := m.visibleListRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
	maxOffset := len(m.items) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset < 0 {
		m.offset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}

// listWindow returns the inclusive-exclusive [start, end) range of items to render.
func (m *scanAddModel) listWindow() (start, end int) {
	m.ensureCursorVisible()
	visible := m.visibleListRows()
	start = m.offset
	end = start + visible
	if end > len(m.items) {
		end = len(m.items)
	}
	return start, end
}

func (m *scanAddModel) View() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Add discovered interfaces to ifc.yaml\n\n")

	selectedStyle := lipgloss.NewStyle().Reverse(true)
	dimStyle := lipgloss.NewStyle().Faint(true)

	switch m.step {
	case scanStepSelect:
		fmt.Fprintf(&builder, "Select interfaces to add (%d found, Space to toggle):\n\n", len(m.items))
		start, end := m.listWindow()
		if start > 0 {
			fmt.Fprintln(&builder, dimStyle.Render(fmt.Sprintf("  ↑ %d more above", start)))
		}
		for i := start; i < end; i++ {
			item := m.items[i]
			check := " "
			if item.selected {
				check = "x"
			}
			line := fmt.Sprintf("  [%s] %s  (%s)", check, item.candidate.Path, item.candidate.Type)
			if i == m.cursor {
				line = selectedStyle.Render(">" + line[1:])
			}
			fmt.Fprintln(&builder, line)
		}
		if end < len(m.items) {
			fmt.Fprintln(&builder, dimStyle.Render(fmt.Sprintf("  ↓ %d more below", len(m.items)-end)))
		}
		fmt.Fprintf(&builder, "\n%d–%d of %d  ·  j/k or ↑/↓ move · pgup/pgdn page · Space toggle · Enter continue",
			start+1, end, len(m.items))
	case scanStepName:
		selected := m.selectedItems()
		if len(selected) == 0 {
			fmt.Fprintf(&builder, "No interfaces selected.\n")
		} else {
			current := selected[m.nameIndex]
			fmt.Fprintf(&builder, "Name for %s (%d of %d):\n\n", current.candidate.Path, m.nameIndex+1, len(selected))
			fmt.Fprint(&builder, m.nameInput.View())
			fmt.Fprintf(&builder, "\n\n↑/↓ or tab switch item · Enter continue")
		}
	case scanStepConfirm:
		fmt.Fprintf(&builder, "Confirm additions:\n\n")
		for _, item := range m.items {
			if !item.selected {
				continue
			}
			fmt.Fprintf(&builder, "  %s  %s  (%s)\n", item.name, item.candidate.Path, item.candidate.Type)
		}
		fmt.Fprintf(&builder, "\nEnter to add · Backspace to edit names")
	}

	if m.errMsg != "" {
		fmt.Fprintf(&builder, "\n\n%s", m.errMsg)
	}
	if m.debugKeys && m.lastKey != "" {
		fmt.Fprintf(&builder, "\n\ndebug: %s", m.lastKey)
	}
	if m.step == scanStepName {
		fmt.Fprintf(&builder, "\n\nctrl+c to quit")
	} else {
		fmt.Fprintf(&builder, "\n\nq to quit")
	}
	return builder.String()
}
