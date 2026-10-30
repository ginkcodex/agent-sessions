package files

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

func GitHistoryCommit(ctx context.Context, req GitHistoryCommitRequest) (GitHistoryCommitDetail, error) {
	var result GitHistoryCommitDetail
	err := withGitHistory(ctx, req.GitHistoryTarget, func(w gitWriter, target gitHistoryResolvedTarget) error {
		var err error
		result, err = readHistoryDetail(w, target, req)
		return err
	})
	return result, err
}

func readHistoryDetail(w gitWriter, target gitHistoryResolvedTarget, req GitHistoryCommitRequest) (GitHistoryCommitDetail, error) {
	result := GitHistoryCommitDetail{Changes: []GitHistoryChange{}}
	length, err := historyOIDLength(w)
	if err != nil {
		return result, err
	}
	if err = validateHistoryMembership(w, req.Tip, req.Commit, length); err != nil {
		return result, err
	}
	metadata, err := readHistoryMetadata(w, []string{req.Commit}, length)
	if err != nil {
		return result, err
	}
	commit := metadata[0]
	result.GitHistorySummary = commit.GitHistorySummary
	result.Committer = commit.Committer
	result.Message = commit.Message
	args := []string{"diff-tree", "--no-commit-id", "--raw", "-z", "-r", "--no-abbrev", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", "-M", "-l500"}
	if len(commit.Parents) == 0 {
		args = append(args, "--root", req.Commit)
	} else {
		result.Base = commit.Parents[0]
		args = append(args, result.Base, req.Commit)
	}
	data, err := w.run("", append(args, "--")...)
	if err != nil {
		return result, err
	}
	result.Changes, result.Truncated, err = parseHistoryChanges(data, target.Scope, length)
	return result, err
}

// parseHistoryChanges consumes raw NUL-framed records without Git's display-path
// quoting. Scope filtering happens before truncation and never leaks rename origins.
func parseHistoryChanges(data []byte, scope string, length int) ([]GitHistoryChange, bool, error) {
	out := []GitHistoryChange{}
	truncated := false
	take := func() (string, error) {
		i := bytes.IndexByte(data, 0)
		if i < 0 {
			return "", errors.New("invalid Git history change record")
		}
		value := string(data[:i])
		data = data[i+1:]
		return value, nil
	}
	for len(data) > 0 {
		header, err := take()
		if err != nil {
			return nil, false, err
		}
		fields := strings.Fields(header)
		if len(fields) != 5 || !strings.HasPrefix(fields[0], ":") || !validHistoryOID(fields[2], length) || !validHistoryOID(fields[3], length) {
			return nil, false, errors.New("invalid Git history change header")
		}
		oldMode := strings.TrimPrefix(fields[0], ":")
		newMode := fields[1]
		for _, mode := range []string{oldMode, newMode} {
			if len(mode) != 6 {
				return nil, false, errors.New("invalid Git file mode")
			}
			if _, err := strconv.ParseUint(mode, 8, 32); err != nil {
				return nil, false, err
			}
		}
		status := fields[4]
		if len(status) == 0 {
			return nil, false, errors.New("missing Git change status")
		}
		first, err := take()
		if err != nil {
			return nil, false, err
		}
		oldPath, newPath := first, first
		if status[0] == 'R' || status[0] == 'C' {
			newPath, err = take()
			if err != nil {
				return nil, false, err
			}
		}
		if !historyTreePath(oldPath) || !historyTreePath(newPath) {
			return nil, false, ErrOutsideRoot
		}
		oldScoped, oldInside := historyScopedPath(oldPath, scope)
		newScoped, newInside := historyScopedPath(newPath, scope)
		change := GitHistoryChange{Path: newScoped, Status: string(status[0]), OldMode: oldMode, NewMode: newMode}
		switch status[0] {
		case 'R':
			if !oldInside && !newInside {
				continue
			}
			switch {
			case !oldInside:
				change.Status = "A"
				change.OldMode = "000000"
			case !newInside:
				change.Path = oldScoped
				change.Status = "D"
				change.NewMode = "000000"
			default:
				change.OldPath = oldScoped
			}
		case 'C':
			if !newInside {
				continue
			}
			change.Status = "A"
			change.OldMode = "000000"
		case 'A', 'M', 'D', 'T':
			if !newInside {
				continue
			}
		default:
			return nil, false, errors.New("unsupported Git change status")
		}
		// JSON cannot round-trip arbitrary Git path bytes. Do not emit a lossy
		// replacement which could alias a different file on the next request.
		// Keep representable siblings usable and disclose the omitted entries.
		if !utf8.ValidString(change.Path) || !utf8.ValidString(change.OldPath) {
			truncated = true
			continue
		}
		if len(out) == MaxGitChanges {
			truncated = true
			continue
		}
		out = append(out, change)
	}
	return out, truncated, nil
}

func historyTreePath(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.ContainsRune(path, 0) {
		return false
	}
	for component := range strings.SplitSeq(path, "/") {
		if component == "" || component == "." || component == ".." || strings.EqualFold(component, ".git") {
			return false
		}
	}
	return true
}

func historyScopedPath(path, scope string) (string, bool) {
	if scope == "" {
		return path, true
	}
	value, ok := strings.CutPrefix(path, scope+"/")
	return value, ok
}
