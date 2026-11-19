package antigravity

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ginkcode/agent-sessions/internal/pathutil"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

// source is one session's transcript.
type source struct {
	ID   string
	Path string
}

// transcriptNames are tried in order. logs/chunks/ mirrors the full
// transcript byte for byte and is never read.
var transcriptNames = []string{"transcript_full.jsonl", "transcript.jsonl"}

// discover lists brain/<id>/.system_generated/logs transcripts, sorted by ID.
// It reads no transcript content. Links leaving the store are rejected.
func (p *Provider) discover(ctx context.Context, diag *provider.Diagnostics) ([]source, error) {
	entries, err := os.ReadDir(p.brainDir())
	if err != nil {
		if !os.IsNotExist(err) {
			diag.Warn(p.brainDir(), 0, "read directory: %v", err)
		}
		return nil, nil
	}
	var sources []source
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := entry.Name()
		if strings.HasPrefix(id, ".") {
			continue
		}
		if src, ok := p.transcript(id, diag); ok {
			sources = append(sources, src)
		}
	}
	return sources, nil
}

// transcript finds session id's transcript under brain/. Links leaving the
// store and non-regular files are rejected with a warning.
func (p *Provider) transcript(id string, diag *provider.Diagnostics) (source, bool) {
	logs := filepath.Join(p.brainDir(), id, ".system_generated", "logs")
	for _, name := range transcriptNames {
		path := filepath.Join(logs, name)
		if _, err := os.Lstat(path); err != nil {
			continue
		}
		if !pathutil.WithinRoot(p.root, path) {
			diag.Warn(path, 0, "transcript symlink escapes the Antigravity root or is broken")
			return source{}, false
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			diag.Warn(path, 0, "not a regular transcript")
			return source{}, false
		}
		return source{ID: id, Path: path}, true
	}
	return source{}, false
}

// sessionID is the brain/<id> directory a transcript path lies in.
func sessionID(path string) string {
	return filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(path))))
}

// firstWorkspace decodes the first URI of a workspace_uris JSON array; agy
// reports that one as the session's cwd.
func firstWorkspace(raw string) string {
	var uris []string
	if json.Unmarshal([]byte(raw), &uris) != nil || len(uris) == 0 {
		return ""
	}
	return workspacePath(uris[0])
}

// workspacePath decodes a file:// URI to a path in its own style:
// file:///home/a%20b is /home/a b, file:///C:/x is C:\x and
// file://server/share is \\server\share. Anything else is "".
func workspacePath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Path == "" {
		return ""
	}
	p := u.Path
	if u.Host != "" && u.Host != "localhost" {
		return pathutil.Clean(`\\` + u.Host + p)
	}
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' && isLetter(p[1]) {
		p = p[1:]
	}
	return pathutil.Clean(p)
}

func isLetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
