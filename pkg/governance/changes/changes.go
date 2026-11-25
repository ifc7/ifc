// Package changes builds, fingerprints and summarises structured change
// records (contract.Change) produced by compare plugins, so every plugin
// reports changes the same way and a governance policy can judge them per
// kind.
package changes

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

// Actions are the last segment of a change kind.
const (
	ActionAdded    = "added"
	ActionRemoved  = "removed"
	ActionModified = "modified"
)

// Kind joins an object, a property and an action into a change kind.
func Kind(object, property, action string) string {
	return object + "." + property + "." + action
}

// Fingerprint is the sha256 of a change's kind and pointer.
func Fingerprint(kind, pointer string) string {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(pointer))
	return hex.EncodeToString(h.Sum(nil))
}

// New builds a change with its default level (error when breaking, warn
// otherwise) and fingerprint.
func New(kind, pointer string, breaking bool, message string) contract.Change {
	level := contract.ChangeLevelWarn
	if breaking {
		level = contract.ChangeLevelError
	}
	return contract.Change{
		Kind:        kind,
		Pointer:     pointer,
		Breaking:    breaking,
		Level:       level,
		Message:     message,
		Fingerprint: Fingerprint(kind, pointer),
	}
}

// Sort orders changes breaking first, then by pointer and kind.
func Sort(cs []contract.Change) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.Breaking != b.Breaking {
			return a.Breaking
		}
		if a.Pointer != b.Pointer {
			return a.Pointer < b.Pointer
		}
		return a.Kind < b.Kind
	})
}

// Counts returns how many changes are breaking and how many are not.
func Counts(cs []contract.Change) (breaking, nonBreaking int) {
	for _, c := range cs {
		if c.Breaking {
			breaking++
		} else {
			nonBreaking++
		}
	}
	return breaking, nonBreaking
}

// RenderText renders changes as compare plugin raw output: one
// "BREAKING|NON-BREAKING kind pointer: message" line per change and a total.
func RenderText(cs []contract.Change) string {
	var b strings.Builder
	for _, c := range cs {
		label := "NON-BREAKING"
		if c.Breaking {
			label = "BREAKING"
		}
		ptr := c.Pointer
		if ptr == "" {
			ptr = "/"
		}
		b.WriteString(label + " " + c.Kind + " " + ptr + ": " + c.Message)
		if c.Stability != "" {
			b.WriteString(" (exempt: " + c.Stability + ")")
		}
		b.WriteString("\n")
	}
	return b.String()
}
