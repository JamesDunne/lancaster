// tarball
package main

import (
	"encoding/binary"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type VirtualTarballReader struct {
	files  tarballFileList
	size   int64
	hashId []byte

	options VirtualTarballOptions

	// Currently open file for reading:
	openFileInfo *TarballFile
	openFile     *os.File
}

func NewVirtualTarballReader(files []*TarballFile, options VirtualTarballOptions) (*VirtualTarballReader, error) {
	t := &VirtualTarballReader{
		files:   tarballFileList(make([]*TarballFile, 0, len(files))),
		options: options,
	}

	uniquePaths := make(map[string]string)
	t.size = int64(0)
	for _, f := range files {
		// Validate paths:
		if filepath.IsAbs(f.Path) {
			return nil, ErrBadPath
		}

		s := strings.Split(f.Path, "/")
		for _, p := range s {
			if p == "." || p == ".." {
				return nil, ErrBadPath
			}
		}

		// Validate LocalPaths:
		if f.LocalPath == "" {
			return nil, ErrMissingLocalPath
		}
		stat, err := os.Lstat(f.LocalPath)
		if err != nil {
			return nil, err
		}
		// TODO: remove this limitation and allow directory entries to have their own permission bits
		if stat.IsDir() {
			return nil, ErrFilesOnly
		}
		if t.options.CompatMode {
			if stat.Mode()&os.ModeType != 0 {
				return nil, ErrCompatViolation
			}
			// Force all chmods to -rw-r--r-- for compatibility purposes:
			f.Mode = 0644
		} else {
			if stat.Mode()&os.ModeSymlink == os.ModeSymlink {
				// Make sure size is 0 since we don't store contents for symlinks:
				f.Size = 0
				// Make sure symlink destination is set:
				if f.SymlinkDestination == "" {
					// Read symlink:
					f.SymlinkDestination, err = os.Readlink(f.LocalPath)
					if err != nil {
						return nil, err
					}
				}
			}
		}

		// Validate all paths are unique:
		if _, ok := uniquePaths[f.Path]; ok {
			return nil, ErrDuplicatePaths
		}
		uniquePaths[f.Path] = f.Path

		// Keep track of the file internally:
		f.offset = t.size
		t.files = append(t.files, f)

		// Each file ends with a terminating NUL character so at least one call to WriteAt or ReadAt will happen to create/read all files.
		t.size += f.Size + 1
	}

	// Sort files for consistency:
	sort.Sort(t.files)

	// Generate a 64-bit hash for identification purposes:
	all := fnv.New64a()
	for _, f := range t.files {
		// Write unique data about file into collection hash:
		all.Write([]byte(f.Path))
		binary.Write(all, byteOrder, f.Size)
		binary.Write(all, byteOrder, f.Mode)
		all.Write([]byte(f.SymlinkDestination))
	}

	// Sum the 64-bit hash:
	t.hashId = make([]byte, 8)
	byteOrder.PutUint64(t.hashId, all.Sum64())

	return t, nil
}

func (t *VirtualTarballReader) HashId() []byte {
	return t.hashId
}

func (t *VirtualTarballReader) closeFile() error {
	if t.openFileInfo == nil {
		t.openFile = nil
		return nil
	}
	if t.openFile == nil {
		t.openFileInfo = nil
		return nil
	}

	if !t.options.CompatMode {
		err := t.openFile.Chmod(t.openFileInfo.Mode)
		if err != nil {
			return err
		}
	}

	err := t.openFile.Close()
	if err != nil {
		return err
	}

	t.openFile = nil
	t.openFileInfo = nil
	return nil
}

// io.Closer:
func (t *VirtualTarballReader) Close() error {
	return t.closeFile()
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
		if offset < tf.offset || offset >= tf.offset+tf.Size+1 {
			continue
		}

		readerAt := io.ReaderAt(nil)
		// Only open normal, non-empty files:
		if tf.Mode&os.ModeType == 0 {
			// Open file if not already:
			if t.openFileInfo != tf {
				// Close and finalize last open file:
				if t.openFileInfo != nil {
					t.closeFile()
				}

				f, err := os.OpenFile(tf.LocalPath, os.O_RDONLY, 0)
				if err != nil {
					return 0, err
				}

				t.openFile = f
				t.openFileInfo = tf
			}

			readerAt = t.openFile
		}

		localOffset := offset - tf.offset
		if localOffset < tf.Size {
			// Perform read from file:
			p := remainder
			if localOffset+int64(len(p)) > tf.Size {
				p = remainder[:tf.Size-localOffset]
			}
			if len(p) > 0 {
				// NOTE: we allow len(p) == 0 as a side effect in case that's useful.
				n, err := readerAt.ReadAt(p, localOffset)
				if err != nil {
					return 0, err
				}

				total += n
				offset += int64(n)
				localOffset += int64(n)
				remainder = remainder[n:]
			}
		}

		// Fill in trailing NUL padding byte:
		if offset == tf.offset+tf.Size && len(remainder) > 0 {
			remainder[0] = 0
			remainder = remainder[1:]
			offset++
			total++
		}

		// Keep iterating files until we have no more to read:
		if len(remainder) == 0 {
			break
		}
	}

	return total, nil
}
