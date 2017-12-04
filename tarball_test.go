package main

import (
	"bytes"
	"os"
	"testing"
)

func newTarball(t *testing.T, files []TarballFile) *Tarball {
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

	// Delete files after test:
	for _, f := range tb.files {
		os.Remove(f.Path)
	}
}

func TestTarball_Nop(t *testing.T) {
	files := []TarballFile{}

	tb := newTarball(t, files)
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

	tb := newTarball(t, files)
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

	tb := newTarball(t, files)
	defer closeTarball(t, tb)

	n, err := tb.WriteAt([]byte("Hello, world!\n"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 14 {
		t.Fatalf("n != 14; n = %v", n)
	}
}

func TestReadAt_OneFile(t *testing.T) {
	wd, err := os.Getwd()
	t.Logf("%s", wd)

	testMessage := []byte("hello, world!\n")
	const fname = "test.txt"

	// Create file for test purposes:
	mainFile, err := os.Stat(fname)
	if os.IsNotExist(err) {
		var file *os.File
		file, err = os.Create(fname)
		file.Write(testMessage)
		file.Close()
		mainFile, err = os.Stat(fname)
	}
	if err != nil {
		t.Fatalf("%v", err)
	}

	if mainFile.Size() != int64(len(testMessage)) {
		t.Fatal("test file size != len(testMessage)")
	}

	files := []TarballFile{
		TarballFile{
			Path: fname,
			Size: mainFile.Size(),
			Mode: mainFile.Mode(),
		},
	}

	tb := newTarball(t, files)
	defer closeTarball(t, tb)

	buf := make([]byte, len(testMessage))
	n, err := tb.ReadAt(buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(testMessage) {
		t.Fatalf("n != %d; n = %v", len(testMessage), n)
	}
	if bytes.Compare(buf, testMessage) != 0 {
		t.Fatalf("test message != read message")
	}
}

func TestReadAt_SpanningFiles(t *testing.T) {
	wd, err := os.Getwd()
	t.Logf("%s", wd)

	testString := "hello, world!\n"
	testMessage := []byte("hello, world!\n")
	const fname = "test1.txt"

	// Create file for test purposes:
	mainFile, err := os.Stat(fname)
	if os.IsNotExist(err) {
		var file *os.File
		file, err = os.Create(fname)
		file.Write(testMessage)
		file.Close()
		mainFile, err = os.Stat(fname)
	}
	if err != nil {
		t.Fatalf("%v", err)
	}

	if mainFile.Size() != int64(len(testMessage)) {
		t.Fatal("test file size != len(testMessage)")
	}

	files := []TarballFile{
		TarballFile{
			Path: fname,
			Size: mainFile.Size(),
			Mode: mainFile.Mode(),
		},
		TarballFile{
			Path: fname,
			Size: mainFile.Size(),
			Mode: mainFile.Mode(),
		},
	}

	tb := newTarball(t, files)
	defer closeTarball(t, tb)

	expectedMessage := []byte(testString + testString)
	expectedLen := len(expectedMessage)
	buf := make([]byte, expectedLen)
	n, err := tb.ReadAt(buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != expectedLen {
		t.Fatalf("n != %d; n = %v", expectedLen, n)
	}
	if bytes.Compare(buf, expectedMessage) != 0 {
		t.Fatalf("expected message != read message")
	}
}