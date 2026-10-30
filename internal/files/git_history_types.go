package files

import "time"

const (
	GitHistoryPageSize        = 50
	MaxGitHistoryPageSize     = 100
	MaxGitHistoryQueryBytes   = 256
	MaxGitHistoryScan         = 100000
	MaxGitHistoryRepositories = 500
	MaxGitHistoryRetained     = 2000
	GitHistoryTimeout         = 20 * time.Second
)

// GitHistoryTarget retains the authorized workspace, independently of the
// selected relative repository. Repo is "." for the containing repository.
type GitHistoryTarget struct {
	Dir  string `json:"dir"`
	Repo string `json:"repo"`
}

type GitHistoryRepository struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

type GitHistoryRepositoryList struct {
	Repositories []GitHistoryRepository `json:"repositories"`
	Truncated    bool                   `json:"truncated"`
}

type GitHistoryRef struct {
	Ref     string `json:"ref"`
	Name    string `json:"name"`
	Remote  bool   `json:"remote"`
	Current bool   `json:"current"`
	OID     string `json:"oid"`
	Kind    string `json:"kind"`
	// Commit is the peeled commit; empty for tags pointing at non-commit objects.
	Commit string `json:"commit"`
}

type GitHistoryRefList struct {
	Refs      []GitHistoryRef `json:"refs"`
	Head      string          `json:"head"`
	HeadRef   string          `json:"headRef"`
	Detached  bool            `json:"detached"`
	Unborn    bool            `json:"unborn"`
	Shallow   bool            `json:"shallow"`
	Truncated bool            `json:"truncated"`
}

type GitHistoryListRequest struct {
	GitHistoryTarget
	Ref    string `json:"ref"`
	Query  string `json:"query"`
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

type GitHistoryPerson struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Time  int64  `json:"time"`
}

type GitHistorySummary struct {
	OID     string           `json:"oid"`
	Subject string           `json:"subject"`
	Author  GitHistoryPerson `json:"author"`
	Parents []string         `json:"parents"`
}

type GitHistoryPage struct {
	Tip     string              `json:"tip"`
	Commits []GitHistorySummary `json:"commits"`
	Cursor  string              `json:"cursor"`
	Limited bool                `json:"limited"`
	Shallow bool                `json:"shallow"`
}

type GitHistoryCommitRequest struct {
	GitHistoryTarget
	Tip    string `json:"tip"`
	Commit string `json:"commit"`
}

// Paths are workspace-relative. A rename crossing the scope boundary is an
// addition/deletion; its outside path and object identity are never returned.
type GitHistoryChange struct {
	Path    string `json:"path"`
	OldPath string `json:"oldPath,omitempty"`
	Status  string `json:"status"`
	OldMode string `json:"oldMode"`
	NewMode string `json:"newMode"`
}

type GitHistoryCommitDetail struct {
	GitHistorySummary
	Committer GitHistoryPerson   `json:"committer"`
	Message   string             `json:"message"`
	Base      string             `json:"base"`
	Changes   []GitHistoryChange `json:"changes"`
	Truncated bool               `json:"truncated"`
}

type GitHistoryDiffRequest struct {
	GitHistoryCommitRequest
	Path string `json:"path"`
}

type GitHistoryDiff struct {
	Content Content          `json:"content"`
	Commit  string           `json:"commit"`
	Base    string           `json:"base"`
	Change  GitHistoryChange `json:"change"`
	Deleted bool             `json:"deleted"`
	// Kind is text, binary, tooLarge, symlink or submodule.
	Kind string `json:"kind"`
}
