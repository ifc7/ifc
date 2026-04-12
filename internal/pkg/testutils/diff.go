package testutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Diff compares two multi-line strings and prints a Diff similar to `git Diff`.
// It returns true if the strings are equal, false otherwise.
func Diff(str1, str2 string) (diffExists bool) {
	if str1 == str2 {
		return false
	}

	lines1 := strings.Split(str1, "\n")
	lines2 := strings.Split(str2, "\n")

	i, j := 0, 0

	// ANSI escape codes for coloring output
	red := "\033[31m"
	green := "\033[32m"
	reset := "\033[0m"

	// Process both strings line by line
	for i < len(lines1) || j < len(lines2) {
		if i < len(lines1) && (j >= len(lines2) || lines1[i] != lines2[j]) {
			fmt.Printf("%s- %s%s\n", red, lines1[i], reset)
			i++
		} else if j < len(lines2) && (i >= len(lines1) || lines1[i] != lines2[j]) {
			fmt.Printf("%s+ %s%s\n", green, lines2[j], reset)
			j++
		} else {
			// If lines are equal, just print them without color
			fmt.Println("  " + lines1[i])
			i++
			j++
		}
	}
	return true
}

// CompareJSON parses two byte strings as JSON and compares them.
func CompareJSON(b1, b2 []byte) (areEqual bool, err error) {
	// Decode JSON
	var j1, j2 interface{}
	err = json.NewDecoder(bytes.NewReader(b1)).Decode(&j1)
	if err != nil {
		return false, fmt.Errorf("failed to decode JSON 1: %w", err)
	}
	err = json.NewDecoder(bytes.NewReader(b2)).Decode(&j2)
	if err != nil {
		return false, fmt.Errorf("failed to decode JSON 2: %w", err)
	}
	// Encode JSON and compare
	m1, err := json.Marshal(j1)
	if err != nil {
		return false, fmt.Errorf("failed to marshal JSON 1: %w", err)
	}
	m2, err := json.Marshal(j2)
	if err != nil {
		return false, fmt.Errorf("failed to marshal JSON 2: %w", err)
	}
	return bytes.Equal(m1, m2), nil
}
