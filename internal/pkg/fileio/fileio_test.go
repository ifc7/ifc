package fileio

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ifc7/ifc/internal/pkg/testutils"
)

func TestWriteFile(t *testing.T) {
	testutils.UseSandbox(t)
	for _, c := range []struct {
		name     string
		filePath string
		fileData []byte
	}{
		{
			name:     "it can write a file to an existing directory",
			filePath: "TestWriteFile-1.txt",
			fileData: []byte("TestWriteFile-1"),
		},
		{
			name:     "it can write a file nested in a directory that does not yet exist",
			filePath: "TestWriteFile-2/TestWriteFile-2.txt",
			fileData: []byte("TestWriteFile-2"),
		},
		{
			name:     "it can write a file nested multiple layers deep in directories that do not yet exist",
			filePath: "TestWriteFile-3/TestWriteFile-3/TestWriteFile-3.txt",
			fileData: []byte("TestWriteFile-3"),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := WriteFile(c.fileData, c.filePath)
			if err != nil {
				t.Fatalf("unexpected error writing file: %v", err)
			}
			data, err := ReadFile(c.filePath)
			if err != nil {
				t.Fatalf("unexpected error reading file: %v", err)
			}
			if string(data) != string(c.fileData) {
				t.Fatalf("expected file data to be %s, got %s", string(c.fileData), string(data))
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	testutils.UseSandbox(t)
	for _, c := range []struct {
		name         string
		filePath     string
		setup        func()
		expectedData []byte
		expectedErr  error
	}{
		{
			name:     "it can read an existing file",
			filePath: "TestReadFile-1.txt",
			setup: func() {
				err := WriteFile([]byte("TestReadFile-1"), "TestReadFile-1.txt")
				if err != nil {
					t.Fatalf("unexpected error writing file: %v", err)
				}
			},
			expectedData: []byte("TestReadFile-1"),
			expectedErr:  nil,
		},
		{
			name:         "it will throw an error if the file does not exist",
			filePath:     "TestReadFile-2.txt",
			setup:        func() {},
			expectedData: nil,
			expectedErr:  fs.ErrNotExist,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.setup()
			data, err := ReadFile(c.filePath)
			if !errors.Is(err, c.expectedErr) {
				t.Fatalf("expected error to be %v, got %v", c.expectedErr, err)
			}
			if string(data) != string(c.expectedData) {
				t.Fatalf("expected file data to be %s, got %s", string(c.expectedData), string(data))
			}
		})
	}
}
