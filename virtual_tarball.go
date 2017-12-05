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
	ErrOutOfRange     = errors.New("Offset out of range")
	ErrNilBuffer      = errors.New("nil buffer")
	ErrBadPAth        = errors.New("bad path")
	ErrDuplicatePaths = errors.New("not all paths are unique")
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
	Path string
	Size int64
	Mode os.FileMode
	Hash []byte
}

type tarballFile struct {
	TarballFile

	offset int64
	writer WriterAtCloser
	reader ReaderAtCloser
}

type tarballFileList []tarballFile

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

	h := sha256.New()
	const bufSize = 4096
	buf := make([]byte, bufSize)
	tn := 0
	for {
		n, err := f.Read(buf)
		if err == io.EOF && n == 0 && tn == 0 {
			return zeroHash[:], nil
		}
		if err != nil {
			return nil, err
		}
		n, err = h.Write(buf[:n])
		// So long as tn != 0 this is sufficient to detect empty hash case.
		tn = n
	}

	return h.Sum(nil), nil
}
