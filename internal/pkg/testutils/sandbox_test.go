package testutils

import (
	"os"
	"testing"

	"github.com/ifc7/ifc/internal/pkg/fileio"
)

func TestUseSandbox(t *testing.T) {
	t.Run("a file can be written to the sandbox during a test", func(t *testing.T) {
		UseSandbox(t)
		// write a file
		err := fileio.WriteFile([]byte("TestFile-1"), "TestFile-1.txt")
		if err != nil {
			t.Fatalf("unexpected error writing file to sandbox: %v", err)
		}
	})
	t.Run("all files should have been cleaned from the sandbox after the test", func(t *testing.T) {
		// check that sandbox folder is empty
		files, err := os.ReadDir(SandboxPath)
		if err != nil {
			t.Fatalf("unexpected error reading sandbox directory: %v", err)
		}
		if len(files) != 0 {
			t.Fatalf("expected sandbox directory to be empty, got %d files", len(files))
		}
	})
}
