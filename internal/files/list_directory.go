package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
)

// MaxDirectoryEntries caps one ListDirectory reply.
const MaxDirectoryEntries = 5000

// DirectoryEntries are the entries of one folder, as slash-separated paths
// relative to the listing root, sorted.
type DirectoryEntries struct {
	Path        string   `json:"path"`
	Files       []string `json:"files"`
	Directories []string `json:"directories"`
	Truncated   bool     `json:"truncated,omitempty"`
}

// ListDirectory reads the entries of the folder rel names under root, without
// walking further: a folder a listing does not look into, such as an ignored
// node_modules, opened in the tree. Like Read it refuses paths that leave
// root. A link to a folder is listed as a folder; version control state is
// left out, as in a listing.
func ListDirectory(ctx context.Context, root, rel string) (DirectoryEntries, error) {
	full, err := Resolve(root, rel)
	if err != nil {
		return DirectoryEntries{}, err
	}
	f, err := os.Open(full)
	if err != nil {
		return DirectoryEntries{}, err
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil {
		return DirectoryEntries{}, err
	} else if !info.IsDir() {
		return DirectoryEntries{}, fmt.Errorf("%s is not a directory", rel)
	}
	out := DirectoryEntries{Path: rel, Files: []string{}, Directories: []string{}}
	for !out.Truncated {
		if err := ctx.Err(); err != nil {
			return DirectoryEntries{}, err
		}
		entries, readErr := f.ReadDir(256)
		for _, e := range entries {
			if len(out.Files)+len(out.Directories) >= MaxDirectoryEntries {
				out.Truncated = true
				break
			}
			dir := e.IsDir()
			if e.Type()&os.ModeSymlink != 0 {
				info, err := os.Stat(filepath.Join(full, e.Name()))
				dir = err == nil && info.IsDir()
			}
			p := path.Join(rel, e.Name())
			switch {
			case dir && vcsDirs[e.Name()]:
			case dir:
				out.Directories = append(out.Directories, p)
			default:
				out.Files = append(out.Files, p)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return DirectoryEntries{}, readErr
		}
	}
	slices.Sort(out.Files)
	slices.Sort(out.Directories)
	return out, nil
}
