package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

// Successln prints a success message with a green checkmark.
func Successln(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(render(Success, "✓") + " " + msg)
}

// Errorln prints an error message with a red cross (to stdout).
func Errorln(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(render(Error, "✗") + " " + msg)
}

// Warnln prints a warning message.
func Warnln(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(render(Warning, "!") + " " + msg)
}

// Infoln prints a muted informational message.
func Infoln(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(render(Muted, "·") + " " + msg)
}

// PrintMessages styles a batch of command result lines by content heuristics.
func PrintMessages(messages []string) {
	for _, message := range messages {
		switch {
		case strings.HasPrefix(message, "Skipped"), strings.Contains(message, "Cancelled"):
			Warnln("%s", message)
		case strings.Contains(message, "up to date"):
			Infoln("%s", message)
		default:
			Successln("%s", message)
		}
	}
}

// StartSpinner shows an animated waiting indicator on stderr until stop is called.
// When stderr is not a TTY (or NO_COLOR is set), it prints a static message once.
func StartSpinner(message string) (stop func()) {
	if os.Getenv("NO_COLOR") != "" ||
		!(isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) {
		fmt.Fprintln(os.Stderr, message)
		return func() {}
	}

	done := make(chan struct{})
	var once sync.Once
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	writeFrame := func(i int) {
		frame := render(Primary, frames[i%len(frames)])
		fmt.Fprintf(os.Stderr, "\r%s %s", frame, render(Muted, message))
	}
	writeFrame(0)

	go func() {
		i := 1
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				fmt.Fprint(os.Stderr, "\r\033[K")
				return
			case <-ticker.C:
				writeFrame(i)
				i++
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
			// Let the goroutine clear the line.
			time.Sleep(100 * time.Millisecond)
		})
	}
}
