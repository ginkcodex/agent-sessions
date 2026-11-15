package files

import (
	"fmt"
	"os"
	"path/filepath"
)

// openRegular keeps project reads contained even if a resolved path is swapped.
func openRegular(root, full string) (*os.File, os.FileInfo, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, err
	}
	name, err := filepath.Rel(realRoot, full)
	if err != nil || !filepath.IsLocal(name) {
		return nil, nil, ErrOutsideRoot
	}
	r, err := os.OpenRoot(realRoot)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	return openRegularAt(r, name)
}

func openRegularAt(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%w: %s is not a regular file", ErrEditTarget, name)
	}
	// Nonblocking open plus fstat closes the regular-file-to-FIFO race.
	f, err := root.OpenFile(name, os.O_RDONLY|regularReadFlags, 0)
	if err != nil {
		return nil, nil, err
	}
	opened, err := f.Stat()
	if err == nil && (!opened.Mode().IsRegular() || !os.SameFile(info, opened)) {
		err = fmt.Errorf("%w: %s changed while opening", ErrEditTarget, name)
	}
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, opened, nil
}
