package fileio

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
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
			filePath := filepath.Join(dir, c.filePath)
			err := WriteFile(c.fileData, filePath)
			if err != nil {
				t.Fatalf("unexpected error writing file: %v", err)
			}
			data, err := ReadFile(filePath)
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
	dir := t.TempDir()
	for _, c := range []struct {
		name         string
		filePath     string
		setup        func()
		expectedData []byte
		expectedErr  error
	}{
		{
			name:     "it can read an existing file",
			filePath: filepath.Join(dir, "TestReadFile-1.txt"),
			setup: func() {
				err := WriteFile([]byte("TestReadFile-1"), filepath.Join(dir, "TestReadFile-1.txt"))
				if err != nil {
					t.Fatalf("unexpected error writing file: %v", err)
				}
			},
			expectedData: []byte("TestReadFile-1"),
			expectedErr:  nil,
		},
		{
			name:         "it will throw an error if the file does not exist",
			filePath:     filepath.Join(dir, "TestReadFile-2.txt"),
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
