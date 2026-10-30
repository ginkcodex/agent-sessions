package files

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"
)

const historyChunkSize = 128

type gitHistoryCursor struct {
	Version   int    `json:"v"`
	Workspace string `json:"workspace"`
	Root      string `json:"root"`
	Repo      string `json:"repo"`
	Scope     string `json:"scope"`
	Ref       string `json:"ref"`
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
	Tip       string `json:"tip"`
	Offset    int    `json:"offset"`
}

// gitHistoryMetadata is shared by list and detail reads. Message is the complete,
// unmodified commit message; Parents includes parents hidden by shallow traversal.
type gitHistoryMetadata struct {
	GitHistorySummary
	Committer GitHistoryPerson
	Message   string
}

func GitHistoryList(ctx context.Context, req GitHistoryListRequest) (GitHistoryPage, error) {
	result := GitHistoryPage{Commits: []GitHistorySummary{}}
	if req.Limit < 0 || req.Limit > MaxGitHistoryPageSize {
		return result, errors.New("invalid Git history page limit")
	}
	if req.Limit == 0 {
		req.Limit = GitHistoryPageSize
	}
	if len(req.Query) > MaxGitHistoryQueryBytes || !utf8.ValidString(req.Query) {
		return result, errors.New("invalid Git history query")
	}
	if req.Ref == "" {
		req.Ref = "HEAD"
	}
	if len(req.Cursor) > 4096 {
		return result, errors.New("invalid Git history cursor")
	}
	err := withGitHistory(ctx, req.GitHistoryTarget, func(w gitWriter, target gitHistoryResolvedTarget) error {
		length, err := historyOIDLength(w)
		if err != nil {
			return err
		}
		if err := validateHistoryRef(w, req.Ref); err != nil {
			return err
		}
		binding := gitHistoryCursor{Version: 1, Workspace: target.Workspace, Root: target.Root, Repo: target.Repo, Scope: target.Scope, Ref: req.Ref, Query: req.Query, Limit: req.Limit}
		cursor := binding
		if req.Cursor != "" {
			data, err := base64.RawURLEncoding.Strict().DecodeString(req.Cursor)
			if err != nil || base64.RawURLEncoding.EncodeToString(data) != req.Cursor {
				return errors.New("invalid Git history cursor")
			}
			cursor = gitHistoryCursor{}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&cursor); err != nil {
				return errors.New("invalid Git history cursor")
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				return errors.New("invalid Git history cursor")
			}
			tip, offset := cursor.Tip, cursor.Offset
			cursor.Tip, cursor.Offset = "", 0
			if cursor != binding || offset <= 0 || offset >= MaxGitHistoryScan || !validHistoryOID(tip, length) {
				return errors.New("mismatched or invalid Git history cursor")
			}
			cursor.Tip, cursor.Offset = tip, offset
			if err := validateHistoryCommit(w, tip, length); err != nil {
				return err
			}
		} else {
			tip, err := w.run("", "rev-parse", "--verify", req.Ref+"^{commit}")
			if err != nil {
				if req.Ref != "HEAD" {
					return err
				}
				// Only an absent symbolic HEAD is unborn, never a broken/missing object.
				unborn, unbornErr := historyHeadUnborn(w)
				if unbornErr != nil {
					return unbornErr
				}
				if !unborn {
					return err
				}
			} else {
				cursor.Tip = strings.TrimSpace(string(tip))
				if err := validateHistoryCommit(w, cursor.Tip, length); err != nil {
					return err
				}
			}
		}
		shallow, err := w.run("", "rev-parse", "--is-shallow-repository")
		if err != nil {
			return err
		}
		result.Shallow = strings.TrimSpace(string(shallow)) == "true"
		result.Tip = cursor.Tip
		if cursor.Tip == "" {
			return nil
		}
		query := strings.ToLower(req.Query)
		initialOffset := cursor.Offset
		for cursor.Offset < MaxGitHistoryScan {
			count := min(historyChunkSize, MaxGitHistoryScan-cursor.Offset)
			// No pathspec here: Git path-history simplification hides relevant merges.
			data, err := w.run("", "rev-list", "--topo-order", "--skip="+strconv.Itoa(cursor.Offset), "--max-count="+strconv.Itoa(count+1), cursor.Tip, "--")
			if err != nil {
				return err
			}
			oids := strings.Fields(string(data))
			if len(oids) == 0 {
				// A supplied offset must point at a commit, not past the end.
				if req.Cursor != "" && cursor.Offset == initialOffset {
					return errors.New("out-of-range Git history cursor")
				}
				return nil
			}
			for _, oid := range oids {
				if !validHistoryOID(oid, length) {
					return errors.New("invalid Git traversal object")
				}
			}
			n := min(count, len(oids))
			commits, err := readHistoryMetadata(w, oids[:n], length)
			if err != nil {
				return err
			}
			for i, commit := range commits {
				cursor.Offset++
				matches := query == "" || strings.Contains(strings.ToLower(commit.Message), query) || strings.Contains(strings.ToLower(commit.Author.Name), query) || strings.Contains(strings.ToLower(commit.Author.Email), query) || historyHashQuery(query) && strings.HasPrefix(commit.OID, query)
				if matches && target.Scope != "" {
					matches, err = historyCommitInScope(w, commit, target.Scope)
					if err != nil {
						return err
					}
				}
				if matches {
					result.Commits = append(result.Commits, commit.GitHistorySummary)
				}
				if cursor.Offset == MaxGitHistoryScan {
					result.Limited = true
					return nil
				}
				if len(result.Commits) == req.Limit {
					if i+1 < len(oids) {
						encoded, err := json.Marshal(cursor)
						if err != nil {
							return err
						}
						result.Cursor = base64.RawURLEncoding.EncodeToString(encoded)
						if len(result.Cursor) > 4096 {
							return errors.New("git history cursor exceeds safety limit")
						}
					}
					return nil
				}
			}
			if len(oids) <= count {
				return nil
			}
		}
		return nil
	})
	return result, err
}

func validateHistoryRef(w gitWriter, ref string) error {
	if ref == "HEAD" {
		return nil
	}
	if !strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/remotes/") && !strings.HasPrefix(ref, "refs/tags/") {
		return errors.New("invalid Git history ref")
	}
	if _, err := w.run("", "check-ref-format", ref); err != nil {
		return fmt.Errorf("invalid Git history ref: %w", err)
	}
	return nil
}

// validateHistoryCommit checks exact object identity and type without revision
// interpretation. Callers must also prove membership before exposing details.
func validateHistoryCommit(w gitWriter, oid string, length int) error {
	if !validHistoryOID(oid, length) {
		return errors.New("invalid Git history object ID")
	}
	data, err := w.run("", "cat-file", "-t", oid)
	if err != nil {
		return err
	}
	if string(data) != "commit\n" {
		return errors.New("git history object is not a commit")
	}
	return nil
}

// validateHistoryMembership is reusable by commit/diff reads. Neither caller
// supplied value reaches Git until both have been validated as full commit OIDs.
func validateHistoryMembership(w gitWriter, tip, commit string, length int) error {
	if err := validateHistoryCommit(w, tip, length); err != nil {
		return err
	}
	if err := validateHistoryCommit(w, commit, length); err != nil {
		return err
	}
	if _, err := w.run("", "merge-base", "--is-ancestor", commit, tip); err != nil {
		return fmt.Errorf("commit is not in pinned Git history: %w", err)
	}
	return nil
}

func historyHashQuery(query string) bool {
	if len(query) < 4 {
		return false
	}
	for _, c := range query {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func historyCommitInScope(w gitWriter, commit gitHistoryMetadata, scope string) (bool, error) {
	args := []string{"diff-tree", "--no-ext-diff", "--no-textconv", "--quiet", "--exit-code", "-r"}
	if len(commit.Parents) == 0 {
		args = append(args, "--root", commit.OID)
	} else {
		args = append(args, commit.Parents[0], commit.OID)
	}
	args = append(args, "--", scope+"/")
	_, err := w.run("", args...)
	if err == nil {
		return false, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, err
}

// readHistoryMetadata uses length-framed raw objects, not delimiter-framed
// messages. One bounded batch serves a whole traversal chunk, including no-query
// repository-wide reads; embedded message delimiters cannot forge records.
func readHistoryMetadata(w gitWriter, oids []string, length int) ([]gitHistoryMetadata, error) {
	for _, oid := range oids {
		if !validHistoryOID(oid, length) {
			return nil, errors.New("invalid Git history object ID")
		}
	}
	data, err := w.run(strings.Join(oids, "\n")+"\n", "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	out := make([]gitHistoryMetadata, 0, len(oids))
	for _, oid := range oids {
		end := bytes.IndexByte(data, '\n')
		if end < 0 {
			return nil, errors.New("invalid Git object record")
		}
		fields := strings.Fields(string(data[:end]))
		data = data[end+1:]
		if len(fields) != 3 || fields[0] != oid || fields[1] != "commit" {
			return nil, errors.New("missing or invalid Git commit object")
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size >= len(data) || data[size] != '\n' {
			return nil, errors.New("invalid Git object size")
		}
		commit, err := parseHistoryCommit(oid, data[:size], length)
		if err != nil {
			return nil, err
		}
		out = append(out, commit)
		data = data[size+1:]
	}
	if len(data) != 0 {
		return nil, errors.New("unexpected Git object records")
	}
	return out, nil
}

func parseHistoryCommit(oid string, raw []byte, length int) (gitHistoryMetadata, error) {
	out := gitHistoryMetadata{GitHistorySummary: GitHistorySummary{OID: oid, Parents: []string{}}}
	if !validHistoryOID(oid, length) || !utf8.Valid(raw) {
		return out, errors.New("invalid Git commit metadata")
	}
	headers, message, ok := strings.Cut(string(raw), "\n\n")
	if !ok {
		return out, errors.New("invalid Git commit headers")
	}
	tree, author, committer := false, false, false
	for line := range strings.SplitSeq(headers, "\n") {
		if strings.HasPrefix(line, " ") {
			continue
		} // signature/extended-header continuations
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "tree":
			if tree || !validHistoryOID(value, length) {
				return out, errors.New("invalid Git commit tree")
			}
			tree = true
		case "parent":
			if !validHistoryOID(value, length) {
				return out, errors.New("invalid Git commit parent")
			}
			out.Parents = append(out.Parents, value)
		case "author", "committer":
			person, err := parseHistoryPerson(value)
			if err != nil {
				return out, err
			}
			if key == "author" {
				if author {
					return out, errors.New("duplicate Git author")
				}
				out.Author = person
				author = true
			} else {
				if committer {
					return out, errors.New("duplicate Git committer")
				}
				out.Committer = person
				committer = true
			}
		}
	}
	if !tree || !author || !committer {
		return out, errors.New("incomplete Git commit metadata")
	}
	out.Message = message
	subject := strings.TrimLeft(message, "\n\r")
	if end := strings.Index(subject, "\n\n"); end >= 0 {
		subject = subject[:end]
	}
	out.Subject = strings.Join(strings.Fields(subject), " ")
	return out, nil
}

func parseHistoryPerson(value string) (GitHistoryPerson, error) {
	out := GitHistoryPerson{}
	start := strings.LastIndex(value, " <")
	end := strings.LastIndex(value, "> ")
	if start < 0 || end < start+2 {
		return out, errors.New("invalid Git commit identity")
	}
	fields := strings.Fields(value[end+2:])
	if len(fields) != 2 {
		return out, errors.New("invalid Git commit timestamp")
	}
	timestamp, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return out, errors.New("invalid Git commit timestamp")
	}
	out.Name = value[:start]
	out.Email = value[start+2 : end]
	out.Time = timestamp
	return out, nil
}
