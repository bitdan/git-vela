package app

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
	"gitvela/internal/git"
)

// RepositoryService exposes read-only operations to the desktop frontend.
type RepositoryService struct{}

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
