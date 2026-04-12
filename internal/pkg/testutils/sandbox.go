package testutils

import (
	"fmt"
	"os"
	"path"
	"testing"
)

var (
	// SandboxPath is the absolute path to the test sandbox directory
	SandboxPath = path.Join(ProjectRoot, testFolder, "sandbox")
)

// UseSandbox will temporarily change the working directory to the sandbox for testing and cleanup and reset the working directory when the test finishes
func UseSandbox(t *testing.T) {
	// create sandbox if it doesn't exist
	_, err := os.Stat(SandboxPath)
	if err != nil {
		err = os.Mkdir(SandboxPath, 0777)
		if err != nil {
			t.Fatalf("unexpected error creating sandbox directory: %v", err)
		}
	}
	resetDir, err := setTempWorkingDir(SandboxPath)
	if err != nil {
		t.Fatalf("unexpected error setting temp working dir: %v", err)
	}
	t.Cleanup(func() {
		cleanSandbox(t)
		if resetDir() != nil {
			t.Fatalf("unexpected error resetting working directory: %v", err)
		}
	})
}

// cleanSandbox will remove the contents of the sandbox directory
// TODO: add some kind of mutex so tests can run in parallel
func cleanSandbox(t *testing.T) {
	err := os.RemoveAll(SandboxPath)
	if err != nil {
		t.Fatalf("unexpected error removing sandbox directory: %v", err)
	}
	err = os.Mkdir(SandboxPath, 0777)
	if err != nil {
		t.Fatalf("unexpected error re-creating sandbox directory: %v", err)
	}
}

// setTempWorkingDir changes the working directory to the given directory and returns a function that can be used to reset the working directory to its original value
func setTempWorkingDir(dir string) (resetDir func() error, err error) {
	originalDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("unexpected error getting working directory: %v", err)
	}
	err = os.Chdir(dir)
	if err != nil {
		return nil, fmt.Errorf("unexpected error changing working directory: %v", err)
	}
	return func() error {
		err = os.Chdir(originalDir)
		if err != nil {
			return fmt.Errorf("unexpected error reseting working directory: %v", err)
		}
		return nil
	}, nil
}
