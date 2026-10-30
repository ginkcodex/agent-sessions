package files

import (
	"context"
	"errors"
	"strings"
)

func ReadGitHistoryDiff(ctx context.Context, req GitHistoryDiffRequest) (GitHistoryDiff, error) {
	var result GitHistoryDiff
	err := withGitHistory(ctx, req.GitHistoryTarget, func(w gitWriter, target gitHistoryResolvedTarget) error {
		detail, err := readHistoryDetail(w, target, req.GitHistoryCommitRequest)
		if err != nil {
			return err
		}
		var change *GitHistoryChange
		for _, candidate := range detail.Changes {
			if candidate.Path == req.Path {
				copy := candidate
				change = &copy
				break
			}
		}
		if change == nil {
			return errors.New("file is not in the selected commit changes")
		}
		result = GitHistoryDiff{Commit: req.Commit, Base: detail.Base, Change: *change, Kind: "text", Deleted: change.Status == "D", Content: Content{Path: req.Path, Edit: &EditInfo{Reason: EditReadOnly}}}
		for _, mode := range []string{change.OldMode, change.NewMode} {
			if mode == "160000" {
				result.Kind = "submodule"
				return nil
			}
			if mode == "120000" {
				result.Kind = "symlink"
				return nil
			}
		}
		oldPath := change.OldPath
		if oldPath == "" {
			oldPath = change.Path
		}
		fullPath := func(path string) string {
			if target.Scope != "" {
				return target.Scope + "/" + path
			}
			return path
		}
		length, err := historyOIDLength(w)
		if err != nil {
			return err
		}
		var oldContent Content
		var oldOID, newOID string
		if change.Status != "A" {
			oldOID, err = historyBlobOID(w, detail.Base, fullPath(oldPath), change.OldMode, length)
			if err != nil {
				return err
			}
			oldContent, err = readGitBlob(w, oldOID, req.Path)
			if err != nil {
				return err
			}
		}
		if change.Status != "D" {
			newOID, err = historyBlobOID(w, req.Commit, fullPath(change.Path), change.NewMode, length)
			if err != nil {
				return err
			}
			result.Content, err = readGitBlob(w, newOID, req.Path)
			if err != nil {
				return err
			}
		}
		result.Content.Binary = result.Content.Binary || oldContent.Binary
		result.Content.TooLarge = result.Content.TooLarge || oldContent.TooLarge
		if result.Content.TooLarge {
			result.Kind = "tooLarge"
			result.Content.Text = ""
			return nil
		}
		if result.Content.Binary {
			result.Kind = "binary"
			result.Content.Text = ""
			return nil
		}
		switch {
		case oldOID != "" && newOID != "":
			patch, err := w.run("", "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--word-diff=none", "--output-indicator-old=-", "--output-indicator-new=+", "--output-indicator-context= ", "-U0", oldOID, newOID, "--")
			if err != nil {
				return err
			}
			result.Content.Changes = parseHunks(string(patch))
		case oldOID != "" && oldContent.Text != "":
			result.Content.Changes = []LineChange{{Kind: "deleted", Start: 0, Removed: historyLines(oldContent.Text)}}
		case newOID != "" && result.Content.Text != "":
			result.Content.Changes = []LineChange{{Kind: "added", Start: 1, Count: len(historyLines(result.Content.Text))}}
		}
		return nil
	})
	return result, err
}

func historyBlobOID(w gitWriter, commit, path, mode string, length int) (string, error) {
	if !validHistoryOID(commit, length) || !historyTreePath(path) {
		return "", ErrOutsideRoot
	}
	if mode != "100644" && mode != "100755" {
		return "", errors.New("unsupported historical file mode")
	}
	data, err := w.run("", "ls-tree", "-z", commit, "--", path)
	if err != nil {
		return "", err
	}
	record := strings.TrimSuffix(string(data), "\x00")
	meta, actual, ok := strings.Cut(record, "\t")
	fields := strings.Fields(meta)
	if !ok || actual != path || len(fields) != 3 || fields[0] != mode || fields[1] != "blob" || !validHistoryOID(fields[2], length) {
		return "", errors.New("historical file is missing or changed identity")
	}
	return fields[2], nil
}

func historyLines(text string) []string {
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}
