package main

import (
	"os"
	"testing"
)

func newTarballWriter(t *testing.T, files []*TarballFile) *VirtualTarballWriter {
	tb, err := NewVirtualTarballWriter(files, getOptions())
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
		verifyFile(t, f, tb)
		os.Remove(f.Path)
	}
}

func verifyFile(t *testing.T, f *TarballFile, tb *VirtualTarballWriter) {
	stat, err := os.Lstat(f.Path)
	if err != nil {
		t.Fatalf("%s", err)
	}
	if stat.Size() != f.Size {
		t.Fatalf("%s: size mistmatch; %d != %d", f.Path, stat.Size(), f.Size)
	}
	if !tb.options.CompatMode {
		if stat.Mode() != f.Mode {
			t.Fatalf("%s: mode mistmatch; %v != %v", f.Path, stat.Mode(), f.Mode)
		}
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

func TestWriteAt_ZeroFile(t *testing.T) {
	files := []*TarballFile{
		&TarballFile{
			Path: "hello.txt",
			Size: 0,
			Mode: 0644,
		},
	}

	tb := newTarballWriter(t, files)
	defer closeTarballWriter(t, tb)

	expectedMessage := []byte("\x00")
	expectedLen := len(expectedMessage)
	n, err := tb.WriteAt(expectedMessage, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != expectedLen {
		t.Fatalf("n != %d; n = %v", expectedLen, n)
	}
}

func TestWriteAt_ZeroFileMultiple(t *testing.T) {
	files := []*TarballFile{
		&TarballFile{
			Path: "hello.txt",
			Size: 0,
			Mode: 0644,
		},
		&TarballFile{
			Path: "hello2.txt",
			Size: 0,
			Mode: 0644,
		},
		&TarballFile{
			Path: "hello3.txt",
			Size: 0,
			Mode: 0644,
		},
		&TarballFile{
			Path: "world.txt",
			Size: 1,
			Mode: 0644,
		},
	}

	tb := newTarballWriter(t, files)
	defer closeTarballWriter(t, tb)

	expectedMessage := []byte("\x00\x00\x00a\x00")
	expectedLen := len(expectedMessage)
	n, err := tb.WriteAt(expectedMessage, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != expectedLen {
		t.Fatalf("n != %d; n = %v", expectedLen, n)
	}
}

func TestWriteAt_ZeroFileMultiple2(t *testing.T) {
	files := []*TarballFile{
		&TarballFile{
			Path: "hello.txt",
			Size: 0,
			Mode: 0644,
		},
		&TarballFile{
			Path: "hello2.txt",
			Size: 0,
			Mode: 0644,
		},
		&TarballFile{
			Path: "world.txt",
			Size: 1,
			Mode: 0644,
		},
		&TarballFile{
			Path: "hello3.txt",
			Size: 0,
			Mode: 0644,
		},
	}

	tb := newTarballWriter(t, files)
	defer closeTarballWriter(t, tb)

	expectedMessage := []byte("\x00\x00a\x00\x00")
	expectedLen := len(expectedMessage)
	n, err := tb.WriteAt(expectedMessage, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != expectedLen {
		t.Fatalf("n != %d; n = %v", expectedLen, n)
	}
}
