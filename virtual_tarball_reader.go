// tarball
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type VirtualTarballReader struct {
	files  tarballFileList
	size   int64
	hashId []byte
}

func NewVirtualTarballReader(files []TarballFile) (*VirtualTarballReader, error) {
	filesInternal := tarballFileList(make([]tarballFile, 0, len(files)))

	all := sha256.New()

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

		// Hash the file's contents:
		h, err := hashFile(f.Path)
		if err != nil {
			return nil, err
		}
		f.Hash = h

		// Write unique data about file into collection hash:
		all.Write([]byte(f.Path))
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, uint64(f.Size))
		all.Write(b)
		all.Write(h)

		// Keep track of the file internally:
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

	return &VirtualTarballReader{
		files:  filesInternal,
		size:   size,
		hashId: all.Sum(nil),
	}, nil
}

func (t *VirtualTarballReader) HashId() []byte {
	return t.hashId
}

// io.Closer:
func (t *VirtualTarballReader) Close() error {
	for _, tf := range t.files {
		if tf.reader != nil {
			err := tf.reader.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// io.ReaderAt:
func (t *VirtualTarballReader) ReadAt(buf []byte, offset int64) (n int, err error) {
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
