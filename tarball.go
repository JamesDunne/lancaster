// tarball
package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
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

type Tarball struct {
	files tarballFileList
	size  int64
}

func NewTarball(files []TarballFile) (*Tarball, error) {
	filesInternal := tarballFileList(make([]tarballFile, 0, len(files)))

	uniquePaths := make(map[string]string)
	size := int64(0)
	for _, f := range files {
		// Validate paths:
		if filepath.IsAbs(f.Path) {
			return nil, ErrBadPAth
		}
		s := strings.Split(f.Path, string(filepath.Separator))
		for _, p := range s {
			if p == "." || p == ".." {
				return nil, ErrBadPAth
			}
		}

		// Validate all paths are unique:
		if _, ok := uniquePaths[f.Path]; ok {
			return nil, ErrDuplicatePaths
		}
		uniquePaths[f.Path] = f.Path

		filesInternal = append(filesInternal, tarballFile{
			TarballFile: f,
			offset:      size,
			writer:      nil,
			reader:      nil,
		})
		size += f.Size
	}

	// Sort files for consistency:
	sort.Sort(filesInternal)

	return &Tarball{
		files: filesInternal,
		size:  size,
	}, nil
}

// io.Closer:
func (t *Tarball) Close() error {
	for _, tf := range t.files {
		if tf.reader != nil {
			err := tf.reader.Close()
			if err != nil {
				return err
			}
		}
		if tf.writer != nil {
			err := tf.writer.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// io.WriterAt:
func (t *Tarball) WriteAt(buf []byte, offset int64) (int, error) {
	if buf == nil {
		return 0, ErrNilBuffer
	}
	if offset < 0 || offset >= t.size {
		return 0, ErrOutOfRange
	}

	// Write to file(s):
	total := 0
	remainder := buf[:]
	for _, tf := range t.files {
		if offset < tf.offset || offset >= tf.offset+tf.Size {
			continue
		}

		// Create file if not already:
		if tf.writer == nil {
			// Try to mkdir all paths involved:
			dir, _ := filepath.Split(tf.Path)
			if dir != "" {
				err := os.MkdirAll(dir, os.FileMode(tf.Mode))
				if err != nil {
					return 0, err
				}
			}

			f, err := os.OpenFile(tf.Path, os.O_WRONLY|os.O_CREATE, os.FileMode(tf.Mode))
			if err != nil {
				return 0, err
			}

			// Reserve disk space:
			err = f.Truncate(tf.Size)
			if err != nil {
				return 0, err
			}

			tf.writer = f
		}

		localOffset := offset - tf.offset

		// Perform write:
		p := remainder
		if localOffset+int64(len(p)) > tf.Size {
			p = remainder[:tf.Size-localOffset]
		}
		if len(p) > 0 {
			// NOTE: we allow len(p) == 0 to create file as a side effect in case that's useful.
			n, err := tf.writer.WriteAt(p, localOffset)
			if err != nil {
				return 0, err
			}
			total += n
			offset += int64(n)
			remainder = remainder[n:]
		}

		// Keep iterating files until we have no more to write:
		if len(remainder) == 0 {
			break
		}
	}

	return total, nil
}

// io.ReaderAt:
func (t *Tarball) ReadAt(buf []byte, offset int64) (n int, err error) {
	if buf == nil {
		return 0, ErrNilBuffer
	}
	if offset < 0 || offset >= t.size {
		return 0, ErrOutOfRange
	}

	// Read from file(s):
	total := 0
	remainder := buf[:]
	for _, tf := range t.files {
		if offset < tf.offset || offset >= tf.offset+tf.Size {
			continue
		}

		// Open file if not already:
		if tf.reader == nil {
			f, err := os.Open(tf.Path)
			if err != nil {
				return 0, err
			}

			tf.reader = f
		}

		localOffset := offset - tf.offset

		// Perform read:
		p := remainder
		if localOffset+int64(len(p)) > tf.Size {
			p = remainder[:tf.Size-localOffset]
		}
		if len(p) > 0 {
			// NOTE: we allow len(p) == 0 as a side effect in case that's useful.
			n, err := tf.reader.ReadAt(p, localOffset)
			if err != nil {
				return 0, err
			}

			total += n
			offset += int64(n)
			remainder = remainder[n:]
		}

		// Keep iterating files until we have no more to read:
		if len(remainder) == 0 {
			break
		}
	}

	return total, nil
}
