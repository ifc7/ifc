// Package fileio provides convenient functions for reading and writing files and directories to a filesystem
// They may not be the most efficient implementations but the focus is on ease of use
package fileio

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteFile writes the given data to a file at the given path. It creates the file if it does not exist, and overwrites it if it does.
// If directories in the file path do not exist, they are created.
func WriteFile(data []byte, filePath string) error {
	fmt.Println("Writing file to", filePath)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		dir := filepath.Dir(filePath)
		err = os.MkdirAll(dir, 0777)
		if err != nil {
			return err
		}
	}
	fo, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() {
		err = fo.Close()
	}()
	w := bufio.NewWriter(fo)
	nn, err := w.Write(data)
	if err != nil {
		return err
	}
	if nn != len(data) {
		return fmt.Errorf("wrote %d bytes, expected %d", nn, len(data))
	}
	if err = w.Flush(); err != nil {
		return err
	}
	return err
}

// ReadFile reads the file at the given path into memory and returns its contents as a byte slice.
// If the file does not exist, an error is returned.
func ReadFile(filePath string) ([]byte, error) {
	fo, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = fo.Close()
	}()
	stat, err := fo.Stat()
	if err != nil {
		return nil, err
	}
	fb := make([]byte, stat.Size())
	_, err = bufio.NewReader(fo).Read(fb)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return fb, err
}

// DirExists checks if a path exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false // Path does not exist
		}
		// Handle other potential errors like permission issues
		fmt.Printf("Error during os.Stat for path %s: %v\n", path, err)
		return false
	}
	return info.IsDir() // Returns true if it is a directory
}

// FileExists checks if a path exists and is a file.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false // Path does not exist
		}
		// Handle other potential errors like permission issues
		fmt.Printf("Error during os.Stat for path %s: %v\n", path, err)
	}
	return !info.IsDir()
}
