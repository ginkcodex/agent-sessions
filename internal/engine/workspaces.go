package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ginkcode/agent-sessions/internal/configfile"
	"github.com/ginkcode/agent-sessions/internal/pathutil"
)

// WorkspaceBackend is optional so older remote hosts remain compatible.
type WorkspaceBackend interface {
	ListWorkspaces(context.Context) ([]string, error)
	OpenWorkspace(context.Context, string) (string, error)
	RemoveWorkspace(context.Context, string) error
	BrowseWorkspace(context.Context, string) (WorkspaceBrowse, error)
}

// WorkspaceBrowse contains absolute host-native paths. Parent equals Path at a volume root.
type WorkspaceBrowse struct {
	Path        string   `json:"path"`
	Parent      string   `json:"parent"`
	Directories []string `json:"directories"`
}

var _ WorkspaceBackend = (*Engine)(nil)

func (e *Engine) ListWorkspaces(_ context.Context) ([]string, error) { return e.svc.listWorkspaces() }
func (e *Engine) OpenWorkspace(_ context.Context, dir string) (string, error) {
	return e.svc.OpenWorkspace(dir)
}
func (e *Engine) RemoveWorkspace(_ context.Context, dir string) error {
	return e.svc.RemoveWorkspace(dir)
}
func (e *Engine) BrowseWorkspace(ctx context.Context, dir string) (WorkspaceBrowse, error) {
	return browseWorkspace(ctx, dir)
}

func canonicalWorkspace(dir string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("workspace must be an absolute directory: %q", dir)
	}
	dir, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", dir)
	}
	return filepath.Clean(dir), nil
}

func (s *Service) workspaceFile() string { return filepath.Join(s.DataDir(), "workspaces.json") }

func (s *Service) listWorkspaces() ([]string, error) {
	out := []string{}
	data, err := os.ReadFile(s.workspaceFile())
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return []string{}, fmt.Errorf("read workspaces: %w", err)
	}
	// Do not require existence here: stale workspaces must remain removable.
	clean := make([]string, 0, len(out))
	seen := make(map[string]bool)
	for _, dir := range out {
		if !filepath.IsAbs(dir) {
			continue
		}
		dir = filepath.Clean(dir)
		key := workspaceKey(dir)
		if !seen[key] {
			clean = append(clean, dir)
			seen[key] = true
		}
	}
	sort.Strings(clean)
	return clean, nil
}

func (s *Service) updateWorkspace(dir string, add bool) error {
	lock, err := configfile.Lock(s.workspaceFile())
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	dirs, err := s.listWorkspaces()
	if err != nil {
		return err
	}
	out := make([]string, 0, len(dirs)+1)
	for _, old := range dirs {
		if workspaceKey(old) != workspaceKey(dir) {
			out = append(out, old)
		}
	}
	if add {
		out = append(out, dir)
	}
	sort.Strings(out)
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return configfile.WriteAtomic(s.workspaceFile(), data)
}

// OpenWorkspace explicitly grants the existing project operations access to this root only.
func (s *Service) OpenWorkspace(dir string) (string, error) {
	dir, err := canonicalWorkspace(dir)
	if err != nil {
		return "", err
	}
	if err := s.updateWorkspace(dir, true); err != nil {
		return "", err
	}
	return dir, nil
}

// RemoveWorkspace only unregisters a root; it never removes filesystem contents or sessions.
func (s *Service) RemoveWorkspace(dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("workspace must be an absolute directory: %q", dir)
	}
	dir = filepath.Clean(dir)
	// Prefer the stored path even if it was replaced by a symlink. A stale
	// or deleted root must still be unregisterable; resolve aliases only
	// when the supplied path is not itself a registration.
	if !s.registeredWorkspace(dir) {
		if resolved, err := canonicalWorkspace(dir); err == nil {
			dir = resolved
		}
	}
	return s.updateWorkspace(dir, false)
}

func workspaceKey(dir string) string {
	dir = pathutil.Clean(dir)
	if pathutil.IsWindows(dir) {
		return strings.ToLower(dir)
	}
	return dir
}

func (s *Service) registeredWorkspace(dir string) bool {
	dirs, err := s.listWorkspaces()
	if err != nil {
		return false
	}
	for _, root := range dirs {
		if workspaceKey(root) == workspaceKey(dir) {
			return true
		}
	}
	return false
}

// Browse is intentionally separate from file access authorization: it exposes directory
// names for explicit user selection, not file contents, and does not register anything.
func browseWorkspace(ctx context.Context, dir string) (WorkspaceBrowse, error) {
	if dir == "" {
		var err error
		dir, err = os.UserHomeDir()
		if err != nil {
			return WorkspaceBrowse{}, err
		}
	}
	dir, err := canonicalWorkspace(dir)
	if err != nil {
		return WorkspaceBrowse{}, err
	}
	out := WorkspaceBrowse{Path: dir, Parent: filepath.Dir(dir), Directories: []string{}}
	f, err := os.Open(dir)
	if err != nil {
		return WorkspaceBrowse{}, err
	}
	defer f.Close()
	// Bound both scanning and response size; ReadDir(n) avoids allocating an unbounded listing.
	const maxEntries = 10000
	const maxDirectories = 1000
	for scanned := 0; scanned < maxEntries && len(out.Directories) < maxDirectories; {
		if err := ctx.Err(); err != nil {
			return WorkspaceBrowse{}, err
		}
		entries, readErr := f.ReadDir(min(256, maxEntries-scanned))
		scanned += len(entries)
		for _, entry := range entries {
			if entry.Name() == ".git" || entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
				continue
			}
			out.Directories = append(out.Directories, filepath.Join(dir, entry.Name()))
			if len(out.Directories) == maxDirectories {
				break
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return WorkspaceBrowse{}, readErr
		}
	}
	sort.Strings(out.Directories)
	return out, nil
}

func (s *Service) workspaceGroupNodes(nodes []GroupNode, filter FilterOpts) []GroupNode {
	dirs, err := s.listWorkspaces()
	if err != nil {
		return nodes
	}
	seen := make(map[string]bool)
	for _, node := range nodes {
		seen[node.Key] = true
	}
	for _, dir := range dirs {
		key := "dir:" + workspaceKey(dir)
		if seen[key] || (filter.Path != "" && !strings.Contains(strings.ToLower(dir), strings.ToLower(filter.Path))) {
			continue
		}
		info, err := os.Stat(dir)
		nodes = append(nodes, GroupNode{Key: key, Label: dir, Secondary: dir, Kind: "directory", CWD: dir, CWDMissing: err != nil || !info.IsDir()})
	}
	sort.SliceStable(nodes, func(i, j int) bool { return strings.ToLower(nodes[i].Label) < strings.ToLower(nodes[j].Label) })
	return nodes
}
