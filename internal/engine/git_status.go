package engine

import (
	"context"
	"fmt"
	"github.com/ginkcode/agent-sessions/internal/files"
)

func (e *Engine) GitStatus(ctx context.Context, dir, repo string) (files.RepoStatus, error) {
	return e.svc.GitStatus(ctx, dir, repo)
}

func (s *Service) GitStatus(ctx context.Context, dir, repo string) (files.RepoStatus, error) {
	r, err := s.gitChangesRepository(ctx, dir, repo)
	if err != nil {
		return files.RepoStatus{}, err
	}
	status, err := files.GitRepoStatus(ctx, r.Root)
	return r.Status(status), err
}

// gitChangesRepository authorizes dir, a session directory, then the
// repository chosen inside it; see files.GitChangesRepository.
func (s *Service) gitChangesRepository(ctx context.Context, dir, repo string) (files.GitChangesRepository, error) {
	if !s.knownDirectory(dir) {
		return files.GitChangesRepository{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, dir)
	}
	return files.ResolveGitChangesRepository(ctx, dir, repo)
}
