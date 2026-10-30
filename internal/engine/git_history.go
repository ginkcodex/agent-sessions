package engine

import (
	"context"
	"fmt"
	"github.com/ginkcode/agent-sessions/internal/files"
)

// GitHistoryBackend is optional and independent of working-tree Git support.
type GitHistoryBackend interface {
	GitHistoryRepositories(context.Context, string) (files.GitHistoryRepositoryList, error)
	GitHistoryBranches(context.Context, files.GitHistoryTarget) (files.GitHistoryRefList, error)
	GitHistoryList(context.Context, files.GitHistoryListRequest) (files.GitHistoryPage, error)
	GitHistoryCommit(context.Context, files.GitHistoryCommitRequest) (files.GitHistoryCommitDetail, error)
	ReadGitHistoryDiff(context.Context, files.GitHistoryDiffRequest) (files.GitHistoryDiff, error)
}

var _ GitHistoryBackend = (*Engine)(nil)

func (e *Engine) GitHistoryRepositories(ctx context.Context, dir string) (files.GitHistoryRepositoryList, error) {
	return e.svc.GitHistoryRepositories(ctx, dir)
}

func (s *Service) GitHistoryRepositories(ctx context.Context, dir string) (files.GitHistoryRepositoryList, error) {
	if !s.knownDirectory(dir) {
		return files.GitHistoryRepositoryList{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, dir)
	}
	return files.GitHistoryRepositories(ctx, dir)
}

func (e *Engine) GitHistoryBranches(ctx context.Context, target files.GitHistoryTarget) (files.GitHistoryRefList, error) {
	return e.svc.GitHistoryBranches(ctx, target)
}

func (s *Service) GitHistoryBranches(ctx context.Context, target files.GitHistoryTarget) (files.GitHistoryRefList, error) {
	if !s.knownDirectory(target.Dir) {
		return files.GitHistoryRefList{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, target.Dir)
	}
	return files.GitHistoryBranches(ctx, target)
}

func (e *Engine) GitHistoryList(ctx context.Context, req files.GitHistoryListRequest) (files.GitHistoryPage, error) {
	return e.svc.GitHistoryList(ctx, req)
}

func (s *Service) GitHistoryList(ctx context.Context, req files.GitHistoryListRequest) (files.GitHistoryPage, error) {
	if !s.knownDirectory(req.Dir) {
		return files.GitHistoryPage{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, req.Dir)
	}
	return files.GitHistoryList(ctx, req)
}

func (e *Engine) GitHistoryCommit(ctx context.Context, req files.GitHistoryCommitRequest) (files.GitHistoryCommitDetail, error) {
	return e.svc.GitHistoryCommit(ctx, req)
}

func (s *Service) GitHistoryCommit(ctx context.Context, req files.GitHistoryCommitRequest) (files.GitHistoryCommitDetail, error) {
	if !s.knownDirectory(req.Dir) {
		return files.GitHistoryCommitDetail{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, req.Dir)
	}
	return files.GitHistoryCommit(ctx, req)
}

func (e *Engine) ReadGitHistoryDiff(ctx context.Context, req files.GitHistoryDiffRequest) (files.GitHistoryDiff, error) {
	return e.svc.ReadGitHistoryDiff(ctx, req)
}

func (s *Service) ReadGitHistoryDiff(ctx context.Context, req files.GitHistoryDiffRequest) (files.GitHistoryDiff, error) {
	if !s.knownDirectory(req.Dir) {
		return files.GitHistoryDiff{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, req.Dir)
	}
	return files.ReadGitHistoryDiff(ctx, req)
}
