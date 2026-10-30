package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrExternalPath is an external path that is not absolute or has a '..'.
var ErrExternalPath = errors.New("external path must be absolute without '..' components")

// ReadExternal reads a file outside any project, named by its absolute path,
// for viewing only: the source a definition in a dependency points to. The
// caller decides which paths may be read; this refuses configured stores and
// protected names (credentials, agent data) as editing does, in the path as
// given and the path it resolves to. Text has the Read limits; Edit and
// Changes are never set, and images are not decoded.
//
// open names directories inside a store that hold no credentials or agent
// data, such as the app's managed tools, whose bundled sources (a type
// checker's standard library stubs) a definition points to. A file inside
// one, as given and as resolved, is read; protected names still refuse.
func ReadExternal(policy EditPolicy, path string, open ...string) (Content, error) {
	if !filepath.IsAbs(path) || editHasParentComponent(path) {
		return Content{}, ErrExternalPath
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Content{}, err
	}
	lex := filepath.Clean(path)
	if err := policy.opening(open, lex, real).refuse(false, lex, real); err != nil {
		return Content{}, err
	}
	f, err := os.Open(real)
	if err != nil {
		return Content{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Content{}, err
	}
	if !info.Mode().IsRegular() {
		return Content{}, fmt.Errorf("%s is not a regular file", path)
	}
	out := Content{Path: path, Size: info.Size()}
	if _, err := readText(f, &out); err != nil {
		return Content{}, err
	}
	return out, nil
}

// opening drops the protected roots holding an open directory that every
// target is inside. An open directory that cannot be resolved opens nothing.
func (p EditPolicy) opening(open []string, targets ...string) EditPolicy {
	for _, dir := range open {
		if !filepath.IsAbs(dir) || editHasParentComponent(dir) {
			continue
		}
		lexOpen := filepath.Clean(dir)
		realOpen, err := editResolveAncestor(lexOpen)
		if err != nil {
			continue
		}
		inside := true
		for _, target := range targets {
			inside = inside && (editWithin(lexOpen, target) || editWithin(realOpen, target))
		}
		if !inside {
			continue
		}
		var kept []string
		for _, protected := range p.ProtectedRoots {
			// A root refuse cannot resolve is kept, so it still fails closed.
			realProtected, err := editResolveAncestor(filepath.Clean(protected))
			if err == nil && (editWithin(filepath.Clean(protected), lexOpen) || editWithin(realProtected, realOpen)) {
				continue
			}
			kept = append(kept, protected)
		}
		p.ProtectedRoots = kept
	}
	return p
}
