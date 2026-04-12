package testutils

import (
	"testing"

	"github.com/ifc7/ifc/internal/pkg/fileio"
)

func TestStrPtr(t *testing.T) {
	s := "test"
	sp := Ptr(s)
	if *sp != s {
		t.Fatalf("expected %s, got %s", s, *sp)
	}
}

func TestMustReadFile(t *testing.T) {
	UseSandbox(t)
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
				err := fileio.WriteFile([]byte("TestReadFile-1"), "TestReadFile-1.txt")
				if err != nil {
					t.Fatalf("unexpected error writing file: %v", err)
				}
			},
			expectedData: []byte("TestReadFile-1"),
			expectedErr:  nil,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.setup()
			data := MustReadFile(t, c.filePath)
			if string(data) != string(c.expectedData) {
				t.Fatalf("expected file data to be %s, got %s", string(c.expectedData), string(data))
			}
		})
	}
}
