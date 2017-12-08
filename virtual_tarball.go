// tarball
package main

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"strings"
)

var (
	ErrOutOfRange       = errors.New("offset out of range")
	ErrNilBuffer        = errors.New("nil buffer")
	ErrBadPAth          = errors.New("bad path")
	ErrDuplicatePaths   = errors.New("not all paths are unique")
	ErrMissingLocalPath = errors.New("missing LocalPath")
	ErrFilesOnly        = errors.New("LocalPaths may only reference files not directories")
	ErrBadPaddingByte   = errors.New("expected 0 padding byte")
)

type ReaderAtCloser interface {
	io.ReaderAt
	io.Closer
}

type WriterAtCloser interface {
	io.WriterAt
	io.Closer
}

type TarballFile struct {
	Path               string
	LocalPath          string
	Size               int64
	Mode               os.FileMode
	SymlinkDestination string

	offset int64
}

type tarballFileList []*TarballFile

func (l tarballFileList) Len() int           { return len(l) }
func (l tarballFileList) Less(i, j int) bool { return strings.Compare(l[i].Path, l[j].Path) == 0 }
func (l tarballFileList) Swap(i, j int) {
	tmpi := l[i]
	l[i] = l[j]
	l[j] = tmpi
}

var zeroHash [32]byte = [32]byte{0}

func hashFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	const bufSize = 4096
	buf := make([]byte, bufSize)
	tn := 0
	for {
		n, err := f.Read(buf)
		if err == io.EOF && n == 0 && tn == 0 {
			return zeroHash[:], nil
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		n, err = h.Write(buf[:n])
		// So long as tn != 0 this is sufficient to detect empty hash case.
		tn = n
	}

	return h.Sum(nil), nil
}
