package engine

import (
	"context"
	"errors"
	"github.com/ginkcode/agent-sessions/internal/files"
	"path/filepath"
	"testing"
)

func TestHistoryActionsExactOriginalDirectoryAuthorization(t *testing.T) {
	root := t.TempDir()
	e := editEngine(root)
	ctx := context.Background()
	for _, dir := range []string{"", t.TempDir(), filepath.Join(root, "child"), filepath.Dir(root)} {
		target := files.GitHistoryTarget{Dir: dir, Repo: root}
		if _, err := e.GitHistoryRefAction(ctx, files.GitHistoryRefActionRequest{GitHistoryTarget: target}); !errors.Is(err, ErrUnknownDirectory) {
			t.Fatal(dir, err)
		}
		req := files.GitHistoryCompareRequest{GitHistoryTarget: target}
		if _, err := e.GitHistoryCompare(ctx, req); !errors.Is(err, ErrUnknownDirectory) {
			t.Fatal(dir, err)
		}
		if _, err := e.ReadGitHistoryCompareDiff(ctx, files.GitHistoryCompareDiffRequest{GitHistoryCompareRequest: req}); !errors.Is(err, ErrUnknownDirectory) {
			t.Fatal(dir, err)
		}
	}
}
