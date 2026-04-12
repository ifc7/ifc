// Package testutils provides utilities for testing SEVEN projects
package testutils

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
)

var (
	_, thisFile, _, _ = runtime.Caller(0)
	// ProjectRoot is the root directory of this project
	ProjectRoot = filepath.Join(filepath.Dir(thisFile), "../..")
	// testFolder contains system-level tests and related data
	testFolder = "test"
	// testdataFolder is the name of the folder where testdata is stored
	testdataFolder = filepath.Join(testFolder, "data")
	// TestdataPath points to where testdata for the entirety of the SEVEN project is stored
	TestdataPath = path.Join(ProjectRoot, testdataFolder)
	// ExampleProjectsPath points to where example projects are stored
	ExampleProjectsPath = path.Join(ProjectRoot, testdataFolder, "example-projects")
)

func Ptr[T any](v T) *T {
	return &v
}

// MustReadFile reads the file at the given path into memory and returns its contents as a byte slice.
// If there is an error reading the file, it fails the test.
func MustReadFile(t *testing.T, filePath string) []byte {
	fo, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("unexpected error opening file: %v", err)
	}
	defer func() {
		err = fo.Close()
	}()
	stat, err := fo.Stat()
	if err != nil {
		t.Fatalf("unexpected error getting file stats: %v", err)
	}
	fb := make([]byte, stat.Size())
	_, err = bufio.NewReader(fo).Read(fb)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error reading file: %v", err)
	}
	return fb
}

// CheckErr checks if the given error matches the expected error.
// If the expected error is nil, it checks that the error is nil.
// It returns a flag indicating whether the test should end after this check.
func CheckErr(t *testing.T, err error, expErr error) (endTest bool) {
	if expErr != nil {
		if err == nil {
			t.Fatal("expected error, got nil")
			return true
		}
		if !errors.Is(err, expErr) {
			t.Fatalf("expected error to be %v, got %v", expErr, err)
			return true
		}
		return true
	}
	if err != nil {
		t.Fatal(err)
		return true
	}
	return false
}
