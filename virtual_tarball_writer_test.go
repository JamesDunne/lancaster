package main

import (
	"os"
	"testing"
)

func newTarballWriter(t *testing.T, files []*TarballFile) *VirtualTarballWriter {
	tb, err := NewVirtualTarballWriter(files)
	if err != nil {
		panic(err)
	}
	return tb
}

func closeTarballWriter(t *testing.T, tb *VirtualTarballWriter) {
	err := tb.Close()
	if err != nil {
		t.Fatalf("Error closing: %v", err)
	}

	// Delete files after test:
	for _, f := range tb.files {
		os.Remove(f.Path)
	}
}

func TestWriteAt_OneFile(t *testing.T) {
	files := []*TarballFile{
		&TarballFile{
			Path: "jim1.txt",
			Size: 3,
			Mode: 0644,
		},
	}

	tb := newTarballWriter(t, files)
	defer closeTarballWriter(t, tb)

	n, err := tb.WriteAt([]byte("hi\n"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatal("n != 2")
	}
}

func TestWriteAt_SpanningFiles(t *testing.T) {
	files := []*TarballFile{
		&TarballFile{
			Path: "hello.txt",
			Size: 7,
			Mode: 0644,
		},
		&TarballFile{
			Path: "world.txt",
			Size: 7,
			Mode: 0644,
		},
	}

	tb := newTarballWriter(t, files)
	defer closeTarballWriter(t, tb)

	expectedMessage := []byte("Hello, \x00world!\n" + "\x00")
	expectedLen := len(expectedMessage)
	n, err := tb.WriteAt(expectedMessage, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != expectedLen {
		t.Fatalf("n != %d; n = %v", expectedLen, n)
	}
}
