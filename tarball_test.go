package main

import (
	"os"
	"testing"
)

func createTarball(t *testing.T, files []TarballFile) *Tarball {
	tb, err := NewTarball(files)
	if err != nil {
		t.Fatalf("NewTarball: %v", err)
	}
	return tb
}

func closeTarball(t *testing.T, tb *Tarball) {
	err := tb.Close()
	if err != nil {
		t.Fatalf("Error closing: %v", err)
	}

	for _, f := range tb.files {
		os.Remove(f.Path)
	}
}

func TestTarball_Nop(t *testing.T) {
	files := []TarballFile{}

	tb := createTarball(t, files)
	defer closeTarball(t, tb)
}

func TestTarball_BadPath1(t *testing.T) {
	files := []TarballFile{
		TarballFile{
			Path: "../test.txt",
		},
	}

	_, err := NewTarball(files)
	if err == nil {
		t.Fatal("Expected non-nil error")
	}
	if err != ErrBadPAth {
		t.Fatal("Expected ErrBadPath")
	}
}

func TestWriteAt_OneFile(t *testing.T) {
	files := []TarballFile{
		TarballFile{
			Path: "jim1.txt",
			Size: 3,
			Mode: 0644,
		},
	}

	tb := createTarball(t, files)
	defer closeTarball(t, tb)

	n, err := tb.WriteAt([]byte("hi\n"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatal("n != 2")
	}
}

func TestWriteAt_SpanningFiles(t *testing.T) {
	files := []TarballFile{
		TarballFile{
			Path: "hello.txt",
			Size: 7,
			Mode: 0644,
		},
		TarballFile{
			Path: "world.txt",
			Size: 7,
			Mode: 0644,
		},
	}

	tb := createTarball(t, files)
	defer closeTarball(t, tb)

	n, err := tb.WriteAt([]byte("Hello, world!\n"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 14 {
		t.Fatalf("n != 14; n = %v", n)
	}
}
