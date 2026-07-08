package datadb

import (
	"bytes"
	"io/fs"
	"strings"
	"time"
)

// memFS is a read-only fs.FS serving a single database image from memory. It
// backs the SQLite VFS in Open: journal / wal / shm siblings are reported
// missing (an immutable read-only database never needs them), and every other
// name returns the image, so SQLite reads the DB entirely from RAM.
type memFS struct{ data []byte }

func (m memFS) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, "-journal") ||
		strings.HasSuffix(name, "-wal") ||
		strings.HasSuffix(name, "-shm") {
		return nil, fs.ErrNotExist
	}
	return &memFile{Reader: bytes.NewReader(m.data), size: int64(len(m.data)), name: name}, nil
}

// memFile adds Stat and Close to a bytes.Reader (which already provides Read and
// Seek, the two the VFS requires).
type memFile struct {
	*bytes.Reader
	size int64
	name string
}

func (f *memFile) Stat() (fs.FileInfo, error) { return memInfo{name: f.name, size: f.size}, nil }
func (f *memFile) Close() error               { return nil }

type memInfo struct {
	name string
	size int64
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return 0o444 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }
