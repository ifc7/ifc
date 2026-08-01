package ui

import (
	"strings"
	"testing"
)

func TestStatusKind_padding(t *testing.T) {
	got := StatusKind("new")
	// Visual width should be 10 regardless of ANSI codes.
	if w := len([]rune(stripANSI(got))); w < 10 {
		// When color is off, plain padded width is 10.
		_ = w
	}
	plain := stripANSI(got)
	if !strings.HasPrefix(strings.TrimRight(plain, " "), "new") {
		t.Fatalf("expected status to start with new, got %q", plain)
	}
	if len(plain) != 10 {
		t.Fatalf("expected padded width 10, got %d (%q)", len(plain), plain)
	}
}

func TestColorUnifiedDiff(t *testing.T) {
	in := "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n context\n"
	out := ColorUnifiedDiff(in)
	plain := stripANSI(out)
	if !strings.Contains(plain, "-old") || !strings.Contains(plain, "+new") {
		t.Fatalf("diff content lost: %q", plain)
	}
}

func TestFormatBreaking(t *testing.T) {
	if stripANSI(FormatBreaking(true)) != "true" {
		t.Fatal("breaking true")
	}
	if stripANSI(FormatBreaking(false)) != "false" {
		t.Fatal("breaking false")
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
