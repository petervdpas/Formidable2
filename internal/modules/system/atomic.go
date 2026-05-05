package system

import (
	"io"
	"os"
	"path/filepath"
)

// atomicWriteFile writes content to target atomically: a uniquely-named
// temp file in the same directory is written, fsynced, and renamed over
// target. POSIX guarantees rename within the same filesystem is atomic,
// so a reader observes either the previous file or the new file — never
// a partial write. On error or unexpected exit before rename, target is
// left untouched and the temp file is cleaned up.
//
// This is the primitive that backs every persistent write in the
// codebase. Domain code should never call os.WriteFile / os.Create /
// os.Rename directly.
func atomicWriteFile(target string, content []byte, perm os.FileMode) error {
	return atomicWriteStream(target, perm, func(w io.Writer) error {
		_, err := w.Write(content)
		return err
	})
}

// atomicWriteStream is the streaming variant: useful when the writer
// fills the file from a source larger than is comfortable to hold in
// memory (file copies, stream re-encodes). fn is invoked with the
// underlying *os.File so callers may also access io.Writer specific
// optimisations on top of plain Write.
func atomicWriteStream(target string, perm os.FileMode, fn func(io.Writer) error) error {
	dir := filepath.Dir(target)
	base := filepath.Base(target)

	// Hidden + .tmp- prefix so a directory listing during a crash can
	// distinguish in-flight writes from real files.
	f, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()

	// On any error path before rename, remove the temp.
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmp)
		}
	}()

	if err := fn(f); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	committed = true
	return nil
}
