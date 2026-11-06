package app

import (
	"context"
	"errors"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"gitvela/internal/git"
)

// RepositoryService exposes Git operations to the desktop frontend.
type RepositoryService struct{ indexWrite sync.Mutex }

// Wails discards successful replies for cancelled calls, but reports late
// rejections as unhandled. A cancelled Git command has no result to deliver.
func discardCancelled[T any](result T, err error) (T, error) {
	if errors.Is(err, context.Canceled) {
		return result, nil
	}
	return result, err
}

func (*RepositoryService) GitVersion(ctx context.Context) (string, error) {
	return git.Version(ctx)
}

func (*RepositoryService) ChooseDirectory() (string, error) {
	return application.Get().Dialog.OpenFile().SetTitle("Open Git Repository").
		CanChooseDirectories(true).CanChooseFiles(false).PromptForSingleSelection()
}

func (*RepositoryService) Open(ctx context.Context, path string) (git.Repository, error) {
	return discardCancelled(git.Open(ctx, path))
}

func (*RepositoryService) History(ctx context.Context, path string, cursor git.Cursor) (git.Page, error) {
	return discardCancelled(git.History(ctx, path, cursor))
}

func (*RepositoryService) Snapshot(ctx context.Context, path string) (string, error) {
	return git.Snapshot(ctx, path)
}

func (*RepositoryService) Files(ctx context.Context, path, commit string, parent int) ([]git.Change, error) {
	return discardCancelled(git.Files(ctx, path, commit, parent))
}

func (*RepositoryService) Diff(ctx context.Context, path, commit string, parent, file int) (git.Comparison, error) {
	return discardCancelled(git.Diff(ctx, path, commit, parent, git.Change{Index: file}))
}

func (*RepositoryService) WorkingChanges(ctx context.Context, path string) ([]git.WorkingFile, error) {
	return discardCancelled(git.WorkingChanges(ctx, path))
}

func (*RepositoryService) WorkingPreview(path, file string) (git.FilePreview, error) {
	return git.WorkingPreview(path, file)
}

func (s *RepositoryService) SetStaged(ctx context.Context, path, file string, staged bool) error {
	s.indexWrite.Lock()
	defer s.indexWrite.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return git.SetStaged(ctx, path, file, staged)
}

func (*RepositoryService) WorkingDiff(ctx context.Context, path, file string, staged bool) (git.FilePreview, error) {
	return discardCancelled(git.WorkingDiff(ctx, path, file, staged))
}

func (s *RepositoryService) CommitIndex(ctx context.Context, path, message string) error {
	s.indexWrite.Lock()
	defer s.indexWrite.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return git.CommitIndex(ctx, path, message)
}

func (s *RepositoryService) CreateBranch(ctx context.Context, path, name, commit string) error {
	s.indexWrite.Lock()
	defer s.indexWrite.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return git.CreateBranch(ctx, path, name, commit)
}

func (s *RepositoryService) SwitchBranch(ctx context.Context, path, name string) error {
	s.indexWrite.Lock()
	defer s.indexWrite.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return git.SwitchBranch(ctx, path, name)
}

func (s *RepositoryService) RenameBranch(ctx context.Context, path, oldName, newName string) error {
	s.indexWrite.Lock()
	defer s.indexWrite.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return git.RenameBranch(ctx, path, oldName, newName)
}

func (s *RepositoryService) DeleteBranch(ctx context.Context, path, name string) error {
	s.indexWrite.Lock()
	defer s.indexWrite.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return git.DeleteBranch(ctx, path, name)
}

func (*RepositoryService) Worktrees(ctx context.Context, path string) ([]git.Worktree, error) {
	return discardCancelled(git.Worktrees(ctx, path))
}

func (s *RepositoryService) AddWorktree(ctx context.Context, path, target, branch, commit string) error {
	s.indexWrite.Lock()
	defer s.indexWrite.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return git.AddWorktree(ctx, path, target, branch, commit)
}

func (s *RepositoryService) RemoveWorktree(ctx context.Context, path, target string) error {
	s.indexWrite.Lock()
	defer s.indexWrite.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return git.RemoveWorktree(ctx, path, target)
}
