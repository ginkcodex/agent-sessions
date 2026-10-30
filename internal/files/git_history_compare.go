package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrGitHistoryCompareStale asks the caller to explicitly refresh its mutable
// comparison rather than silently displaying content from a different snapshot.
var ErrGitHistoryCompareStale = errors.New("working tree or selected ref changed; refresh comparison")

type GitHistoryCompareRequest struct {
	GitHistoryTarget
	Ref         string `json:"ref"`
	ExpectedOID string `json:"expectedOID"`
}

type GitHistoryComparison struct {
	Ref       string             `json:"ref"`
	OID       string             `json:"oid"`
	Commit    string             `json:"commit"`
	Token     string             `json:"token"`
	Changes   []GitHistoryChange `json:"changes"`
	Truncated bool               `json:"truncated"`
}

type GitHistoryCompareDiffRequest struct {
	GitHistoryCompareRequest
	Token string `json:"token"`
	Path  string `json:"path"`
}

type GitHistoryCompareDiff struct {
	Content Content          `json:"content"`
	Ref     string           `json:"ref"`
	OID     string           `json:"oid"`
	Commit  string           `json:"commit"`
	Token   string           `json:"token"`
	Change  GitHistoryChange `json:"change"`
	Deleted bool             `json:"deleted"`
	Kind    string           `json:"kind"`
}

func GitHistoryCompare(ctx context.Context, req GitHistoryCompareRequest) (GitHistoryComparison, error) {
	var result GitHistoryComparison
	err := withHistoryComparison(ctx, req, func(w gitWriter, target gitHistoryResolvedTarget) error {
		var err error
		result, err = historyComparison(w, target, req)
		return err
	})
	return result, err
}

// Resolve the ORIGINAL authorized target again after acquiring the common-dir
// lock. Never replace Dir with a discovered repository root.
func withHistoryComparison(ctx context.Context, req GitHistoryCompareRequest, fn func(gitWriter, gitHistoryResolvedTarget) error) error {
	return withGitHistory(ctx, req.GitHistoryTarget, func(w gitWriter, before gitHistoryResolvedTarget) error {
		after, err := resolveGitHistoryTarget(w.ctx, req.GitHistoryTarget)
		if err != nil {
			return err
		}
		if before != after {
			return ErrGitHistoryCompareStale
		}
		return fn(w, after)
	})
}

func historyComparison(w gitWriter, target gitHistoryResolvedTarget, req GitHistoryCompareRequest) (GitHistoryComparison, error) {
	out := GitHistoryComparison{Ref: req.Ref, OID: req.ExpectedOID, Changes: []GitHistoryChange{}}
	length, err := historyOIDLength(w)
	if err != nil {
		return out, err
	}
	if len(req.Ref) > 1024 || req.Ref == "HEAD" {
		return out, errors.New("comparison requires a full selected ref")
	}
	if err := validateHistoryRef(w, req.Ref); err != nil {
		return out, err
	}
	if !validHistoryOID(req.ExpectedOID, length) {
		return out, errors.New("invalid expected ref object ID")
	}
	direct, err := w.run("", "rev-parse", "--verify", req.Ref)
	if err != nil {
		return out, err
	}
	if strings.TrimSpace(string(direct)) != req.ExpectedOID {
		return out, ErrGitHistoryCompareStale
	}
	peeled, err := w.run("", "rev-parse", "--verify", req.ExpectedOID+"^{commit}")
	if err != nil {
		return out, err
	}
	out.Commit = strings.TrimSpace(string(peeled))
	if err := validateHistoryCommit(w, out.Commit, length); err != nil {
		return out, err
	}
	data, err := w.run("", "diff", "--raw", "-z", "--no-abbrev", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", "-M", "-l500", out.Commit, "--")
	if err != nil {
		return out, err
	}
	changes, truncated, err := parseHistoryChanges(data, target.Scope, length)
	if err != nil {
		return out, err
	}
	out.Truncated = truncated
	others, err := w.run("", "ls-files", "--others", "--exclude-standard", "-z", "--")
	if err != nil {
		return out, err
	}
	byPath := map[string]GitHistoryChange{}
	for _, change := range changes {
		byPath[change.Path] = change
	}
	for _, path := range strings.Split(string(others), "\x00") {
		if path == "" {
			continue
		}
		if !historyTreePath(path) {
			return out, ErrOutsideRoot
		}
		scoped, inside := historyScopedPath(path, target.Scope)
		if !inside {
			continue
		}
		if !utf8.ValidString(scoped) {
			out.Truncated = true
			continue
		}
		if previous, ok := byPath[scoped]; ok {
			// A staged deletion can leave a recreated (untracked) saved file behind.
			if previous.Status == "D" {
				previous.Status = "M"
				previous.NewMode = "100644"
				byPath[scoped] = previous
			}
			continue
		}
		if len(byPath) >= MaxGitChanges {
			out.Truncated = true
			continue
		}
		byPath[scoped] = GitHistoryChange{Path: scoped, Status: "A", OldMode: "000000", NewMode: "100644"}
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	root, err := os.OpenRoot(target.Root)
	if err != nil {
		return out, err
	}
	defer root.Close()
	for _, path := range paths {
		change := byPath[path]
		// Protected components are excluded before content/stat collection too.
		full := historyComparePath(target, path)
		if err := (EditPolicy{}).refuse(false, filepath.Join(target.Root, filepath.FromSlash(full)), filepath.Join(target.Root, filepath.FromSlash(full))); err != nil {
			out.Truncated = true
			continue
		}
		if change.OldPath != "" {
			old := filepath.Join(target.Root, filepath.FromSlash(historyComparePath(target, change.OldPath)))
			if err := (EditPolicy{}).refuse(false, old, old); err != nil {
				out.Truncated = true
				continue
			}
		}
		// A staged deletion may be recreated with an ignored name, so it
		// will not necessarily appear in ls-files --others above.
		if change.Status == "D" && change.OldMode != "160000" {
			if _, err := root.Lstat(filepath.FromSlash(full)); err == nil {
				change.Status = "M"
			} else if !errors.Is(err, os.ErrNotExist) {
				return out, err
			}
		}
		if change.Status != "D" {
			info, err := root.Lstat(filepath.FromSlash(full))
			if err != nil {
				return out, err
			}
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				change.NewMode = "120000"
			case info.IsDir():
				change.NewMode = "160000"
			case info.Mode().IsRegular():
				change.NewMode = "100644"
				if info.Mode()&0111 != 0 {
					change.NewMode = "100755"
				}
			default:
				out.Truncated = true
				continue
			}
		}
		out.Changes = append(out.Changes, change)
	}
	out.Token, err = historyCompareToken(w, target, out, root)
	return out, err
}

func historyComparePath(target gitHistoryResolvedTarget, path string) string {
	if target.Scope != "" {
		return target.Scope + "/" + path
	}
	return path
}

// The token is target/ref-bound and hashes normal sized displayed files. Large
// files use size/mtime/mode identity (their contents are never displayed). The
// aggregate hashing budget bounds work even for a list of many large files.
func historyCompareToken(w gitWriter, target gitHistoryResolvedTarget, list GitHistoryComparison, root *os.Root) (string, error) {
	h := sha256.New()
	binding := struct {
		Target gitHistoryResolvedTarget
		List   GitHistoryComparison
	}{target, list}
	data, _ := json.Marshal(binding)
	h.Write(data)
	budget := int64(32 << 20)
	for _, change := range list.Changes {
		if err := w.ctx.Err(); err != nil {
			return "", err
		}
		if change.Status == "D" {
			continue
		}
		path := filepath.FromSlash(historyComparePath(target, change.Path))
		info, err := root.Lstat(path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00%d\x00", path, info.Size(), info.ModTime().UnixNano(), info.Mode())
		if !info.Mode().IsRegular() {
			continue
		}
		// Refuse symlink parents even when they happen to point inside the root.
		if err := gitDiscardPreflight(EditPolicy{}, target.Root, filepath.ToSlash(path)); err != nil {
			return "", err
		}
		if info.Size() > MaxReadBytes || info.Size() > budget {
			continue
		}
		f, err := root.Open(path)
		if err != nil {
			return "", err
		}
		n, copyErr := io.Copy(h, io.LimitReader(f, info.Size()+1))
		closeErr := f.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if n != info.Size() {
			return "", ErrGitHistoryCompareStale
		}
		budget -= n
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ReadGitHistoryCompareDiff(ctx context.Context, req GitHistoryCompareDiffRequest) (GitHistoryCompareDiff, error) {
	var out GitHistoryCompareDiff
	if len(req.Token) != 64 || !historyTreePath(req.Path) || !utf8.ValidString(req.Path) {
		return out, errors.New("invalid comparison read request")
	}
	if _, err := hex.DecodeString(req.Token); err != nil {
		return out, errors.New("invalid comparison token")
	}
	err := withHistoryComparison(ctx, req.GitHistoryCompareRequest, func(w gitWriter, target gitHistoryResolvedTarget) error {
		list, err := historyComparison(w, target, req.GitHistoryCompareRequest)
		if err != nil {
			return err
		}
		if list.Token != req.Token {
			return ErrGitHistoryCompareStale
		}
		var selected *GitHistoryChange
		for _, change := range list.Changes {
			if change.Path == req.Path {
				copy := change
				selected = &copy
				break
			}
		}
		if selected == nil {
			return errors.New("file is not in selected working-tree comparison")
		}
		out = GitHistoryCompareDiff{Ref: list.Ref, OID: list.OID, Commit: list.Commit, Token: list.Token, Change: *selected, Deleted: selected.Status == "D", Kind: "text", Content: Content{Path: req.Path, Edit: &EditInfo{Reason: EditReadOnly}}}
		if err := readHistoryComparisonFile(w, target, list, *selected, &out); err != nil {
			return err
		}
		after, err := historyComparison(w, target, req.GitHistoryCompareRequest)
		if err != nil {
			return err
		}
		if after.Token != req.Token {
			return ErrGitHistoryCompareStale
		}
		return nil
	})
	if err != nil {
		return GitHistoryCompareDiff{}, err
	}
	return out, nil
}

func readHistoryComparisonFile(w gitWriter, target gitHistoryResolvedTarget, list GitHistoryComparison, change GitHistoryChange, out *GitHistoryCompareDiff) error {
	for _, mode := range []string{change.OldMode, change.NewMode} {
		if mode == "160000" {
			out.Kind = "submodule"
			return nil
		}
		if mode == "120000" {
			out.Kind = "symlink"
			return nil
		}
	}
	var old Content
	if change.Status != "A" {
		path := change.OldPath
		if path == "" {
			path = change.Path
		}
		length, err := historyOIDLength(w)
		if err != nil {
			return err
		}
		oid, err := historyBlobOID(w, list.Commit, historyComparePath(target, path), change.OldMode, length)
		if err != nil {
			return err
		}
		old, err = readGitBlob(w, oid, change.Path)
		if err != nil {
			return err
		}
	}
	if !out.Deleted {
		path := historyComparePath(target, change.Path)
		if err := gitDiscardPreflight(EditPolicy{}, target.Root, path); err != nil {
			return err
		}
		root, err := os.OpenRoot(target.Root)
		if err != nil {
			return err
		}
		defer root.Close()
		f, err := root.Open(filepath.FromSlash(path))
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return ErrEditTarget
		}
		out.Content.Size = info.Size()
		_, err = readText(f, &out.Content)
		if err != nil {
			return err
		}
		out.Content.Revision = ""
	}
	out.Content.Binary = out.Content.Binary || old.Binary
	out.Content.TooLarge = out.Content.TooLarge || old.TooLarge
	if out.Content.TooLarge {
		out.Kind = "tooLarge"
		out.Content.Text = ""
		return nil
	}
	if out.Content.Binary {
		out.Kind = "binary"
		out.Content.Text = ""
		return nil
	}
	changes, err := historyCompareText(w, old.Text, out.Content.Text)
	if err != nil {
		return err
	}
	out.Content.Changes = changes
	return nil
}

// Diff bounded saved snapshots, not the live disk (which may change between the
// content and patch reads). No hash-object/write-tree/index writes are needed.
func historyCompareText(w gitWriter, old, current string) ([]LineChange, error) {
	if old == current {
		return nil, nil
	}
	if old == "" {
		return []LineChange{{Kind: "added", Start: 1, Count: len(historyLines(current))}}, nil
	}
	if current == "" {
		return []LineChange{{Kind: "deleted", Start: 0, Removed: historyLines(old)}}, nil
	}
	dir, err := os.MkdirTemp("", "agent-sessions-history-compare-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	a, b := filepath.Join(dir, "old"), filepath.Join(dir, "new")
	if err := os.WriteFile(a, []byte(old), 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(b, []byte(current), 0600); err != nil {
		return nil, err
	}
	// --no-index exits 1 when differences exist; use a dedicated bounded runner.
	cmd := exec.CommandContext(w.ctx, w.git, "--no-pager", "diff", "--no-index", "--no-color", "--no-ext-diff", "--no-textconv", "--word-diff=none", "--output-indicator-old=-", "--output-indicator-new=+", "--output-indicator-context= ", "-U0", "--", a, b)
	hideConsole(cmd)
	prepareGitWriteProcess(cmd)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), gitHistoryEnv()...)
	stdout := &gitLimitedBuffer{limit: 16 << 20}
	stderr := &gitLimitedBuffer{limit: gitErrorLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if w.ctx.Err() != nil {
		return nil, w.ctx.Err()
	}
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return nil, fmt.Errorf("compare saved text: %w: %s", err, stderr.buffer.String())
	}
	if stdout.overflow {
		return nil, errors.New("comparison patch exceeds safety limit")
	}
	return parseHunks(stdout.buffer.String()), nil
}
