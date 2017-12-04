// tarball
package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

var (
	ErrOutOfRange = errors.New("Offset out of range")
	ErrNilBuffer  = errors.New("nil buffer")
)

type TarballFile struct {
	Path string
	Size int64
	Mode int32
}

type tarballFile struct {
	TarballFile

	offset int64
	f      *os.File
}

type Tarball struct {
	files []tarballFile
	size  int64
}

func NewTarball(files []TarballFile) *Tarball {
	filesInternal := make([]tarballFile, 0, len(files))

	size := int64(0)
	for _, f := range files {
		filesInternal = append(filesInternal, tarballFile{
			TarballFile: f,
			offset:      size,
			f:           nil,
		})
		size += f.Size
	}

	return &Tarball{
		files: filesInternal,
		size:  size,
	}
}

// io.WriterAt:
func (t *Tarball) WriteAt(buf []byte, offset int64) (int, error) {
	if offset < 0 || offset >= t.size {
		return 0, ErrOutOfRange
	}
	if buf == nil {
		return 0, ErrNilBuffer
	}

	// Write to file(s):
	total := 0
	remainder := buf[:]
	for _, tf := range t.files {
		if offset < tf.offset || offset >= tf.offset+tf.Size {
			continue
		}

		var err error

		// Create file if not existing:
		if tf.f == nil {
			tf.f, err = os.OpenFile(tf.Path, os.O_WRONLY|os.O_CREATE, os.FileMode(tf.Mode))
			if err != nil {
				return 0, err
			}

			// Reserve disk space:
			err = tf.f.Truncate(tf.Size)
			if err != nil {
				return 0, err
			}
		}

		localOffset := offset - tf.offset

		// Perform write:
		p := remainder
		if localOffset+int64(len(p)) > tf.Size {
			p = remainder[:tf.Size-localOffset]
		}
		fmt.Fprintf(os.Stderr, "write('%s'): %s", tf.Path, hex.Dump(p))
		if len(p) > 0 {
			// NOTE: we allow len(p) == 0 to create file as a side effect in case that's useful.
			n, err := tf.f.WriteAt(p, localOffset)
			if err != nil {
				return 0, err
			}
			total += n
			offset += int64(n)
			remainder = remainder[n:]
		}

		fmt.Fprintf(os.Stderr, "remain: %s", hex.Dump(remainder))

		if len(remainder) == 0 {
			break
		}
	}

	return total, nil
}
