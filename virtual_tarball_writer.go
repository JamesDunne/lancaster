// tarball
package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type VirtualTarballWriter struct {
	files  tarballFileList
	size   int64
	hashId []byte
}

func NewVirtualTarballWriter(files []TarballFile, hashId []byte) (*VirtualTarballWriter, error) {
	filesInternal := tarballFileList(make([]*tarballFile, 0, len(files)))

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

		filesInternal = append(filesInternal, &tarballFile{
			TarballFile: f,
			offset:      size,
			writer:      nil,
			reader:      nil,
		})
		size += f.Size
	}

	// Sort files for consistency:
	sort.Sort(filesInternal)

	return &VirtualTarballWriter{
		files:  filesInternal,
		size:   size,
		hashId: hashId,
	}, nil
}

func (t *VirtualTarballWriter) HashId() []byte {
	return t.hashId
}

// io.Closer:
func (t *VirtualTarballWriter) Close() error {
	for _, tf := range t.files {
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
func (t *VirtualTarballWriter) WriteAt(buf []byte, offset int64) (int, error) {
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
				// Make sure directories are at least rwx by owner:
				err := os.MkdirAll(dir, tf.Mode|0700)
				if err != nil {
					return 0, err
				}
			}

			f, err := os.OpenFile(tf.Path, os.O_WRONLY|os.O_CREATE, tf.Mode)
			if err != nil {
				if os.IsPermission(err) {
					// If permission denied, attempt to add -w- to owner:
					tf.Mode |= 0200
					f, err = os.OpenFile(tf.Path, os.O_WRONLY|os.O_CREATE, tf.Mode)
					if err != nil {
						return 0, err
					}
				} else {
					return 0, err
				}
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
