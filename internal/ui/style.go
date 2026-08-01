// Package ui provides shared terminal styling aligned with the ifc-web brand.
package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

// Brand colors from ifc-web / docs/logo.svg.
const (
	hexPrimary      = "#6C63FF"
	hexPrimaryLight = "#5B52F4"
	hexSuccess      = "#00D27F"
	hexSuccessLight = "#0F766E"
	hexAccent       = "#C6F221"
	hexError        = "#EF4444"
	hexErrorLight   = "#DC2626"
	hexWarning      = "#F59E0B"
	hexWarningLight = "#D97706"
	hexMuted        = "#9CA3AF"
	hexMutedLight   = "#6B7280"
	hexEmphasis     = "#F5F5F7"
)

var (
	colorPrimary = lipgloss.AdaptiveColor{Light: hexPrimaryLight, Dark: hexPrimary}
	colorSuccess = lipgloss.AdaptiveColor{Light: hexSuccessLight, Dark: hexSuccess}
	colorAccent  = lipgloss.AdaptiveColor{Light: hexSuccessLight, Dark: hexAccent}
	colorError   = lipgloss.AdaptiveColor{Light: hexErrorLight, Dark: hexError}
	colorWarning = lipgloss.AdaptiveColor{Light: hexWarningLight, Dark: hexWarning}
	colorMuted   = lipgloss.AdaptiveColor{Light: hexMutedLight, Dark: hexMuted}
)

// Exported styles for reuse across commands and TUI views.
var (
	Primary  = lipgloss.NewStyle().Foreground(colorPrimary)
	Success  = lipgloss.NewStyle().Foreground(colorSuccess)
	Accent   = lipgloss.NewStyle().Foreground(colorAccent)
	Error    = lipgloss.NewStyle().Foreground(colorError)
	Warning  = lipgloss.NewStyle().Foreground(colorWarning)
	Muted    = lipgloss.NewStyle().Foreground(colorMuted)
	Emphasis = lipgloss.NewStyle().Bold(true)
	Title    = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	Label    = lipgloss.NewStyle().Foreground(colorMuted)
)

// ColorEnabled reports whether ANSI styling should be applied.
func ColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICOLOR_FORCE") == "1" {
		return true
	}
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

func render(style lipgloss.Style, s string) string {
	if !ColorEnabled() {
		return s
	}
	return style.Render(s)
}

// Apply renders s with style when color output is enabled.
func Apply(style lipgloss.Style, s string) string {
	return render(style, s)
}

// Wordmark returns a short branded mark for headers.
func Wordmark() string {
	return render(Title, "ifc")
}

// ScreenTitle renders "ifc · <title>" with a divider, for TUI screens.
func ScreenTitle(title string) string {
	head := Wordmark() + render(Muted, "  ·  ") + render(Emphasis, title)
	return head + "\n" + Divider() + "\n"
}

// Divider is a faint horizontal rule.
func Divider() string {
	return render(Muted, "────────────────────────")
}

// Section renders a section heading inside a TUI step.
func Section(text string) string {
	return render(Label, text)
}

// KeyHints renders footer keybind help.
func KeyHints(text string) string {
	return render(Muted, text)
}

// Field renders a "Label: value" confirmation row.
func Field(label, value string) string {
	return render(Label, label+":") + " " + value
}

// ListRow renders a selectable checkbox row.
func ListRow(selected, checked bool, label string) string {
	cursor := "  "
	if selected {
		cursor = render(Primary, "> ")
	}
	check := render(Muted, "[ ]")
	if checked {
		check = render(Success, "[x]")
	}
	if selected {
		label = render(Emphasis, label)
	}
	return cursor + check + " " + label
}

// StatusKind styles a status label (clean/modified/new/missing/error) to width 10.
func StatusKind(kind string) string {
	var style lipgloss.Style
	switch kind {
	case "new":
		style = Success
	case "modified":
		style = Primary
	case "missing":
		style = Warning
	case "error":
		style = Error
	case "clean":
		style = Muted
	default:
		style = lipgloss.NewStyle()
	}
	colored := render(style, kind)
	pad := 10 - lipgloss.Width(colored)
	if pad < 0 {
		pad = 0
	}
	return colored + strings.Repeat(" ", pad)
}

// ColorUnifiedDiff colors +/−/header lines of a unified diff.
func ColorUnifiedDiff(diff string) string {
	if diff == "" || !ColorEnabled() {
		return diff
	}
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if line == "" && i == len(lines)-1 {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			lines[i] = render(Muted, line)
		case strings.HasPrefix(line, "@@"):
			lines[i] = render(Primary, line)
		case strings.HasPrefix(line, "+"):
			lines[i] = render(Success, line)
		case strings.HasPrefix(line, "-"):
			lines[i] = render(Error, line)
		}
	}
	return strings.Join(lines, "\n")
}

// FormatScore colors a lint quality score.
func FormatScore(score int) string {
	s := fmt.Sprintf("%d", score)
	switch {
	case score >= 80:
		return render(Success, s)
	case score >= 50:
		return render(Warning, s)
	default:
		return render(Error, s)
	}
}

// FormatBreaking colors a compare breaking flag.
func FormatBreaking(breaking bool) string {
	if breaking {
		return render(Warning, "true")
	}
	return render(Success, "false")
}

// ApplyTextInput applies brand prompt/placeholder styles to a text input.
func ApplyTextInput(ti *textinput.Model) {
	ti.Prompt = "> "
	ti.PromptStyle = Primary
	ti.TextStyle = Emphasis
	ti.PlaceholderStyle = Muted
	ti.Cursor.Style = Primary
}
