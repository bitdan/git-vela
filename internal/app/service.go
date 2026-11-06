package app

import (
	"context"
	"errors"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"gitvela/internal/git"
)

// RepositoryService exposes Git operations to the desktop frontend.
type RepositoryService struct {
	indexWrite sync.Mutex
	watchMu    sync.Mutex
	stopWatch  func()
}

const repositoryChanged = "gitvela:repository-changed"

func init() {
	application.RegisterEvent[string](repositoryChanged)
}

func (s *RepositoryService) WatchRepository(ctx context.Context, path string) error {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if ctx.Err() != nil {
		return nil
	}
	if s.stopWatch != nil {
		s.stopWatch()
		s.stopWatch = nil
	}
	stop, err := git.WatchRepository(ctx, path, func() {
		application.Get().Event.Emit(repositoryChanged, path)
	})
	if ctx.Err() != nil {
		if stop != nil {
			stop()
		}
		return nil
	}
	if err != nil {
		return err
	}
	s.stopWatch = stop
	return nil
}

func (s *RepositoryService) ServiceShutdown() error {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.stopWatch != nil {
		s.stopWatch()
		s.stopWatch = nil
	}
	return nil
}

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
	return application.Get().Dialog.OpenFile().SetTitle("Select a Folder").
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

// DiffExpanded is only called after the user confirms loading a large preview.
func (*RepositoryService) DiffExpanded(ctx context.Context, path, commit string, parent, file int) (git.Comparison, error) {
	return discardCancelled(git.DiffExpanded(ctx, path, commit, parent, git.Change{Index: file}))
}

func (*RepositoryService) WorkingChanges(ctx context.Context, path string) ([]git.WorkingFile, error) {
	return discardCancelled(git.WorkingChanges(ctx, path))
}

func (*RepositoryService) WorkingPreview(path, file string) (git.FilePreview, error) {
	return git.WorkingPreview(path, file)
}

func (*RepositoryService) WorkingPreviewExpanded(path, file string) (git.FilePreview, error) {
	return git.WorkingPreviewExpanded(path, file)
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

func (*RepositoryService) WorkingDiffExpanded(ctx context.Context, path, file string, staged bool) (git.FilePreview, error) {
	return discardCancelled(git.WorkingDiffExpanded(ctx, path, file, staged))
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

func (*RepositoryService) SearchHistory(ctx context.Context, path string, filter git.HistoryFilter, cursor git.Cursor) (git.Page, error) {
	return discardCancelled(git.SearchHistory(ctx, path, filter, cursor))
}

func (*RepositoryService) CommitMessage(ctx context.Context, path, hash string) (string, error) {
	return discardCancelled(git.ReadCommitMessage(ctx, path, hash))
}

func (s *RepositoryService) write(ctx context.Context, action func() error) error {
	s.indexWrite.Lock()
	defer s.indexWrite.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return action()
}

func (*RepositoryService) PrepareAmend(ctx context.Context, path string) (git.AmendTarget, error) {
	return discardCancelled(git.PrepareAmend(ctx, path))
}

func (s *RepositoryService) AmendIndex(ctx context.Context, path, expectedHead, expectedRef, message string) error {
	return s.write(ctx, func() error { return git.AmendIndex(ctx, path, expectedHead, expectedRef, message) })
}

func (*RepositoryService) Stashes(ctx context.Context, path string) ([]git.StashEntry, error) {
	return discardCancelled(git.Stashes(ctx, path))
}

func (s *RepositoryService) SaveStash(ctx context.Context, path, message string, untracked bool) error {
	return s.write(ctx, func() error { return git.SaveStash(ctx, path, message, untracked) })
}

func (s *RepositoryService) UpdateStash(ctx context.Context, path, action, selector, hash string, restoreIndex bool) error {
	return s.write(ctx, func() error { return git.UpdateStash(ctx, path, action, selector, hash, restoreIndex) })
}

func (*RepositoryService) SyncState(ctx context.Context, path string) (git.SyncState, error) {
	return discardCancelled(git.ReadSyncState(ctx, path))
}

func (s *RepositoryService) FetchRemote(ctx context.Context, path, remote string) error {
	return s.write(ctx, func() error { return git.FetchRemote(ctx, path, remote) })
}

func (s *RepositoryService) PullUpstream(ctx context.Context, path, branch, remote, target string) error {
	return s.write(ctx, func() error { return git.PullUpstream(ctx, path, branch, remote, target) })
}

func (s *RepositoryService) PushBranch(ctx context.Context, path, branch, remote, target string) error {
	return s.write(ctx, func() error { return git.PushBranch(ctx, path, branch, remote, target) })
}

func (s *RepositoryService) Initialize(ctx context.Context, path string) (git.Repository, error) {
	var repo git.Repository
	err := s.write(ctx, func() error {
		var err error
		repo, err = git.Initialize(ctx, path)
		return err
	})
	return repo, err
}

func (s *RepositoryService) StageAll(ctx context.Context, path string) error {
	return s.write(ctx, func() error { return git.StageAll(ctx, path) })
}

func (*RepositoryService) Remotes(ctx context.Context, path string) ([]git.RemoteInfo, error) {
	return discardCancelled(git.ReadRemotes(ctx, path))
}

func (s *RepositoryService) AddRemote(ctx context.Context, path, name, address string) error {
	return s.write(ctx, func() error { return git.AddRemote(ctx, path, name, address) })
}

func (s *RepositoryService) SetRemoteURL(ctx context.Context, path, name, address string) error {
	return s.write(ctx, func() error { return git.SetRemoteURL(ctx, path, name, address) })
}

func (s *RepositoryService) RemoveRemote(ctx context.Context, path, name string) error {
	return s.write(ctx, func() error { return git.RemoveRemote(ctx, path, name) })
}

func (*RepositoryService) GitHubLogin(ctx context.Context, path string) (string, error) {
	return discardCancelled(git.GitHubLogin(ctx, path))
}

func (s *RepositoryService) PublishGitHub(ctx context.Context, path string, request git.PublishRequest) (git.PublishResult, error) {
	var result git.PublishResult
	err := s.write(ctx, func() error {
		var err error
		result, err = git.PublishGitHub(ctx, path, request)
		return err
	})
	return result, err
}

func (*RepositoryService) EditableFiles(ctx context.Context, path string) ([]string, error) {
	return discardCancelled(git.EditableFilePaths(ctx, path))
}

func (*RepositoryService) OpenTextFile(ctx context.Context, path, file string) (git.TextFile, error) {
	return discardCancelled(git.OpenTextFile(ctx, path, file))
}

func (s *RepositoryService) SaveTextFile(ctx context.Context, path, file, revision, content string) (string, error) {
	var updated string
	err := s.write(ctx, func() error {
		var err error
		updated, err = git.SaveTextFile(ctx, path, file, revision, content)
		return err
	})
	return updated, err
}

func (*RepositoryService) FileHistory(ctx context.Context, path, file string) (git.FileHistoryPage, error) {
	return discardCancelled(git.FileHistory(ctx, path, file))
}

func (*RepositoryService) FileBlame(ctx context.Context, path, file string) ([]git.BlameLine, error) {
	return discardCancelled(git.FileBlame(ctx, path, file))
}

func (*RepositoryService) CompareFiles(ctx context.Context, path, before, after string) ([]git.Change, error) {
	return discardCancelled(git.CompareFiles(ctx, path, before, after))
}

func (*RepositoryService) CompareDiff(ctx context.Context, path, before, after string, index int) (git.Comparison, error) {
	return discardCancelled(git.CompareDiff(ctx, path, before, after, index))
}

func (*RepositoryService) CompareDiffExpanded(ctx context.Context, path, before, after string, index int) (git.Comparison, error) {
	return discardCancelled(git.CompareDiffExpanded(ctx, path, before, after, index))
}

func (*RepositoryService) ReadHead(ctx context.Context, path string) (git.HeadState, error) {
	return discardCancelled(git.ReadHead(ctx, path))
}

func (*RepositoryService) OperationState(ctx context.Context, path string) (git.OperationState, error) {
	return discardCancelled(git.ReadOperationState(ctx, path))
}

func (s *RepositoryService) ApplyHistoryOperation(ctx context.Context, path, action, target string, expected git.HeadState, mode string) error {
	return s.write(ctx, func() error { return git.ApplyHistoryOperation(ctx, path, action, target, expected, mode) })
}

func (s *RepositoryService) FinishHistoryOperation(ctx context.Context, path, kind string, abort bool) error {
	return s.write(ctx, func() error { return git.FinishHistoryOperation(ctx, path, kind, abort) })
}

func (s *RepositoryService) ResolveConflict(ctx context.Context, path, file string) error {
	return s.write(ctx, func() error { return git.ResolveConflict(ctx, path, file) })
}

func (*RepositoryService) WorkingHunks(ctx context.Context, path, file string, staged bool) (git.HunkPatch, error) {
	return discardCancelled(git.WorkingHunks(ctx, path, file, staged))
}

func (s *RepositoryService) SetHunkStaged(ctx context.Context, path, file string, staged bool, revision string, index int) error {
	return s.write(ctx, func() error { return git.SetHunkStaged(ctx, path, file, staged, revision, index) })
}

func (s *RepositoryService) SetHunkLinesStaged(ctx context.Context, path, file string, staged bool, revision string, index int, lines []int) error {
	return s.write(ctx, func() error { return git.SetHunkLinesStaged(ctx, path, file, staged, revision, index, lines) })
}

func (*RepositoryService) ConflictVersions(ctx context.Context, path, file string) ([]git.ConflictVersion, error) {
	return discardCancelled(git.ConflictVersions(ctx, path, file))
}

func (*RepositoryService) RebasePlan(ctx context.Context, path, base string, expected git.HeadState) ([]git.Commit, error) {
	return discardCancelled(git.ReadRebasePlan(ctx, path, base, expected))
}

func (s *RepositoryService) RunInteractiveRebase(ctx context.Context, path, base string, expected git.HeadState, steps []git.RebaseStep) error {
	return s.write(ctx, func() error { return git.RunInteractiveRebase(ctx, path, base, expected, steps) })
}

func (*RepositoryService) SubmodulePath(ctx context.Context, path, file string) (string, error) {
	return discardCancelled(git.SubmodulePath(ctx, path, file))
}
