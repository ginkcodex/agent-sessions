package app

import (
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

func (a *App) GitHistoryRepositories(dir string) (files.GitHistoryRepositoryList, error) {
	r := a.route()
	if offline, ok := r.backend.(offlineBackend); ok {
		return files.GitHistoryRepositoryList{}, offline.err()
	}
	b, ok := r.backend.(engine.GitHistoryBackend)
	if !ok {
		return files.GitHistoryRepositoryList{}, engine.ErrUnsupportedAction
	}
	return b.GitHistoryRepositories(a.appCtx(), dir)
}

func (a *App) GitHistoryBranches(target files.GitHistoryTarget) (files.GitHistoryRefList, error) {
	r := a.route()
	if offline, ok := r.backend.(offlineBackend); ok {
		return files.GitHistoryRefList{}, offline.err()
	}
	b, ok := r.backend.(engine.GitHistoryBackend)
	if !ok {
		return files.GitHistoryRefList{}, engine.ErrUnsupportedAction
	}
	return b.GitHistoryBranches(a.appCtx(), target)
}

func (a *App) GitHistoryList(req files.GitHistoryListRequest) (files.GitHistoryPage, error) {
	r := a.route()
	if offline, ok := r.backend.(offlineBackend); ok {
		return files.GitHistoryPage{}, offline.err()
	}
	b, ok := r.backend.(engine.GitHistoryBackend)
	if !ok {
		return files.GitHistoryPage{}, engine.ErrUnsupportedAction
	}
	return b.GitHistoryList(a.appCtx(), req)
}

func (a *App) GitHistoryCommit(req files.GitHistoryCommitRequest) (files.GitHistoryCommitDetail, error) {
	r := a.route()
	if offline, ok := r.backend.(offlineBackend); ok {
		return files.GitHistoryCommitDetail{}, offline.err()
	}
	b, ok := r.backend.(engine.GitHistoryBackend)
	if !ok {
		return files.GitHistoryCommitDetail{}, engine.ErrUnsupportedAction
	}
	return b.GitHistoryCommit(a.appCtx(), req)
}

func (a *App) ReadGitHistoryDiff(req files.GitHistoryDiffRequest) (files.GitHistoryDiff, error) {
	r := a.route()
	if offline, ok := r.backend.(offlineBackend); ok {
		return files.GitHistoryDiff{}, offline.err()
	}
	b, ok := r.backend.(engine.GitHistoryBackend)
	if !ok {
		return files.GitHistoryDiff{}, engine.ErrUnsupportedAction
	}
	return b.ReadGitHistoryDiff(a.appCtx(), req)
}
