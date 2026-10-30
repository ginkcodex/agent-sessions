package files

import (
	"strings"
)

// gitSubmoduleMode is the mode Git records a submodule, a gitlink, with.
const gitSubmoduleMode = "160000"

// gitSubmoduleCommit returns the commit the index, or HEAD's tree when head is
// set, records for the submodule at rel; "" when rel is not one there.
func gitSubmoduleCommit(w gitWriter, rel string, head bool) (string, error) {
	args := []string{"ls-files", "--stage", "-z", "--", rel}
	if head {
		if _, err := w.run("", "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
			return "", nil
		}
		args = []string{"ls-tree", "-z", "HEAD", "--", rel}
	}
	out, err := w.run("", args...)
	if err != nil {
		return "", err
	}
	for _, record := range strings.Split(string(out), "\x00") {
		meta, path, ok := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		// ls-files: <mode> <object> <stage>; ls-tree: <mode> <type> <object>.
		if !ok || path != rel || len(fields) != 3 || fields[0] != gitSubmoduleMode {
			continue
		}
		if head {
			return fields[2], nil
		}
		return fields[1], nil
	}
	return "", nil
}

// readGitSubmoduleDiff reads a submodule's change as git diff shows it: one
// line naming the commit it is at, ending in -dirty when its files changed.
// ok is false when rel is not a submodule in the index or HEAD.
func readGitSubmoduleDiff(w gitWriter, rel string, basis GitDiffBasis) (result GitDiffContent, ok bool, err error) {
	result = GitDiffContent{Basis: basis, ReadOnly: true, Submodule: true}
	indexed, err := gitSubmoduleCommit(w, rel, false)
	if err != nil {
		return result, false, err
	}
	committed, err := gitSubmoduleCommit(w, rel, true)
	if err != nil || (indexed == "" && committed == "") {
		return result, false, err
	}
	args := []string{"diff", "--no-color", "--no-ext-diff", "--submodule=short", "--ignore-submodules=none"}
	switch basis {
	case GitDiffHead:
		if committed != "" {
			args = append(args, "HEAD")
		}
	case GitDiffStaged:
		args = append(args, "--cached")
		if committed != "" {
			args = append(args, "HEAD")
		}
	}
	out, err := w.run("", append(args, "--", rel)...)
	if err != nil {
		return result, false, err
	}
	var removed, added string
	for _, line := range strings.Split(string(out), "\n") {
		if text, found := strings.CutPrefix(line, "-Subproject commit "); found {
			removed = "Subproject commit " + text
		} else if text, found := strings.CutPrefix(line, "+Subproject commit "); found {
			added = "Subproject commit " + text
		}
	}
	text := added
	var changes []LineChange
	switch {
	case removed != "" && added != "":
		changes = []LineChange{{Kind: "modified", Start: 1, Count: 1, Removed: []string{removed}}}
	case added != "":
		changes = []LineChange{{Kind: "added", Start: 1, Count: 1}}
	case removed != "":
		result.Deleted = true
		changes = []LineChange{{Kind: "deleted", Start: 0, Count: 0, Removed: []string{removed}}}
	default:
		// No change on this basis: the commit both sides record.
		commit := indexed
		if commit == "" {
			commit = committed
		}
		text = "Subproject commit " + commit
	}
	if text != "" {
		text += "\n"
	}
	result.Content = Content{Path: rel, Size: int64(len(text)), Text: text, Changes: changes, Edit: &EditInfo{Reason: EditReadOnly}}
	return result, true, nil
}
