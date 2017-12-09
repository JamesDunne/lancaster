package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"runtime"
	"testing"
)

func getOptions() VirtualTarballOptions {
	options := VirtualTarballOptions{}
	if runtime.GOOS == "windows" {
		options.CompatMode = true
	}
	return options
}

func newTarballReader(t *testing.T, files []*TarballFile) *VirtualTarballReader {
	tb, err := NewVirtualTarballReader(files, getOptions())
	if err != nil {
		panic(err)
	}
	return tb
}

func closeTarballReader(t *testing.T, tb *VirtualTarballReader) {
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
	files := []*TarballFile{}

	tb := newTarballReader(t, files)
	defer closeTarballReader(t, tb)
}

func TestTarball_BadPath1(t *testing.T) {
	files := []*TarballFile{
		&TarballFile{
			Path: "../test.txt",
		},
	}

	_, err := NewVirtualTarballReader(files, getOptions())
	if err == nil {
		t.Fatal("Expected non-nil error")
	}
	if err != ErrBadPath {
		t.Fatal("Expected ErrBadPath")
	}
}

func TestReadAt_OneFile(t *testing.T) {
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

	files := []*TarballFile{
		&TarballFile{
			Path:      fname,
			LocalPath: fname,
			Size:      mainFile.Size(),
			Mode:      mainFile.Mode(),
		},
	}

	tb := newTarballReader(t, files)
	defer closeTarballReader(t, tb)

	expectedBytes := []byte(string(testMessage) + "\x00")
	expectedLen := len(expectedBytes)
	buf := make([]byte, expectedLen)
	n, err := tb.ReadAt(buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != expectedLen {
		t.Fatalf("n != %d; n = %v", expectedLen, n)
	}
	if bytes.Compare(buf, expectedBytes) != 0 {
		t.Fatalf("test message != read message")
	}
}

func createTestFile(path string, contents []byte) (os.FileInfo, error) {
	// Create file for test purposes:
	mainFile, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = ioutil.WriteFile(path, contents, os.FileMode(0644))
		if err != nil {
			return nil, err
		}
		mainFile, err = os.Stat(path)
	}
	return mainFile, err
}

func TestReadAt_SpanningFiles(t *testing.T) {
	testString := "hello, world!\n"
	testMessage := []byte("hello, world!\n")
	const fname1 = "test1.txt"
	const fname2 = "test2.txt"

	// Create file for test purposes:
	testFile1, err := createTestFile(fname1, testMessage)
	if err != nil {
		t.Fatalf("%v", err)
	}
	testFile2, err := createTestFile(fname2, testMessage)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if testFile1.Size() != int64(len(testMessage)) {
		t.Fatal("test file size != len(testMessage)")
	}

	if testFile2.Size() != int64(len(testMessage)) {
		t.Fatal("test file size != len(testMessage)")
	}

	files := []*TarballFile{
		&TarballFile{
			Path:      fname1,
			LocalPath: fname1,
			Size:      testFile1.Size(),
			Mode:      testFile1.Mode(),
		},
		&TarballFile{
			Path:      fname2,
			LocalPath: fname2,
			Size:      testFile2.Size(),
			Mode:      testFile2.Mode(),
		},
	}

	tb := newTarballReader(t, files)
	defer closeTarballReader(t, tb)

	expectedMessage := []byte(testString + "\x00" + testString + "\x00")
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
