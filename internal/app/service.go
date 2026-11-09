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

func (*RepositoryService) InspectBranchDeletion(ctx context.Context, path, name string) (git.BranchDeletion, error) {
	return discardCancelled(git.InspectBranchDeletion(ctx, path, name))
}

func (s *RepositoryService) DeleteBranch(ctx context.Context, path, name, expectedTip string, force bool) error {
	return s.write(ctx, func() error { return git.DeleteBranch(ctx, path, name, expectedTip, force) })
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

func (*RepositoryService) FirstParent(ctx context.Context, path, hash string) (string, error) {
	return discardCancelled(git.FirstParent(ctx, path, hash))
}

// write serializes Git write commands for the whole process. Keeping one lock
// is deliberate: the UI drives one repository at a time, linked worktrees share
// refs and the index lock of their common directory, and a per-repository lock
// would add path canonicalization for no user-visible gain. Read calls do not
// take it.
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

func (s *RepositoryService) AmendIndex(ctx context.Context, path, expectedHead, expectedRef, message string, options git.AmendOptions) error {
	return s.write(ctx, func() error { return git.AmendIndex(ctx, path, expectedHead, expectedRef, message, options) })
}

func (*RepositoryService) CommitTemplate(ctx context.Context, path string) (string, error) {
	return discardCancelled(git.CommitTemplate(ctx, path))
}

func (*RepositoryService) Stashes(ctx context.Context, path string) ([]git.StashEntry, error) {
	return discardCancelled(git.Stashes(ctx, path))
}

func (s *RepositoryService) SaveStash(ctx context.Context, path, message string, untracked bool, paths []string) error {
	return s.write(ctx, func() error { return git.SaveStash(ctx, path, message, untracked, paths) })
}

func (s *RepositoryService) RenameStash(ctx context.Context, path string, entry git.StashEntry, message string) error {
	return s.write(ctx, func() error { return git.RenameStash(ctx, path, entry, message) })
}

func (s *RepositoryService) UpdateStash(ctx context.Context, path, action, selector, hash string, restoreIndex bool) error {
	return s.write(ctx, func() error { return git.UpdateStash(ctx, path, action, selector, hash, restoreIndex) })
}

func (*RepositoryService) SyncState(ctx context.Context, path string) (git.SyncState, error) {
	return discardCancelled(git.ReadSyncState(ctx, path))
}

func (*RepositoryService) CheckRepositoryPath(path string) git.RepositoryAvailability {
	return git.CheckRepositoryPath(path)
}

func (s *RepositoryService) FetchRemote(ctx context.Context, path, remote string, options git.FetchOptions) error {
	return s.write(ctx, func() error { return git.FetchRemote(ctx, path, remote, options) })
}

func (s *RepositoryService) FetchAllRemotes(ctx context.Context, path string, options git.FetchOptions) error {
	return s.write(ctx, func() error { return git.FetchAllRemotes(ctx, path, options) })
}

func (s *RepositoryService) BisectState(ctx context.Context, path string) (git.BisectState, error) {
	return discardCancelled(git.ReadBisectState(ctx, path))
}

func (s *RepositoryService) StartBisect(ctx context.Context, path, bad, good string) error {
	return s.write(ctx, func() error { return git.StartBisect(ctx, path, bad, good) })
}

func (s *RepositoryService) MarkBisect(ctx context.Context, path, verdict string) error {
	return s.write(ctx, func() error { return git.MarkBisect(ctx, path, verdict) })
}

func (s *RepositoryService) ResetBisect(ctx context.Context, path string) error {
	return s.write(ctx, func() error { return git.ResetBisect(ctx, path) })
}

func (s *RepositoryService) RunGC(ctx context.Context, path string) error {
	return s.write(ctx, func() error { return git.RunGC(ctx, path) })
}

func (*RepositoryService) UnreachableCommits(ctx context.Context, path string) (int, error) {
	return discardCancelled(git.UnreachableCommits(ctx, path))
}

// ConfigureGit switches the Git binary for the whole process. The version of
// the selected binary is returned so the settings form can show it.
func (*RepositoryService) ConfigureGit(ctx context.Context, path string) (string, error) {
	return git.ConfigureExecutable(ctx, path)
}

func (*RepositoryService) BranchTracking(ctx context.Context, path, branch string) (git.BranchTracking, error) {
	return discardCancelled(git.ReadBranchTracking(ctx, path, branch))
}

// UpdateBranch runs the branch "Update" action on an explicit local branch.
func (s *RepositoryService) UpdateBranch(ctx context.Context, path, branch, method string) (git.UpdateResult, error) {
	return writeValue(s, ctx, func() (git.UpdateResult, error) { return git.UpdateBranch(ctx, path, branch, method) })
}

// Pull integrates an explicitly selected remote branch into the checked-out branch.
func (s *RepositoryService) Pull(ctx context.Context, path string, request git.PullRequest) (git.UpdateResult, error) {
	return writeValue(s, ctx, func() (git.UpdateResult, error) { return git.Pull(ctx, path, request) })
}

// writeValue runs a value-returning write under the write lock.
func writeValue[T any](s *RepositoryService, ctx context.Context, action func() (T, error)) (T, error) {
	var result T
	err := s.write(ctx, func() (err error) {
		result, err = action()
		return err
	})
	return result, err
}

func (s *RepositoryService) PushBranch(ctx context.Context, path, branch, remote, target string, options git.PushOptions) error {
	return s.write(ctx, func() error { return git.PushBranch(ctx, path, branch, remote, target, options) })
}

func (*RepositoryService) BranchSync(ctx context.Context, path string) ([]git.BranchSync, error) {
	return discardCancelled(git.BranchSyncCounts(ctx, path))
}

func (s *RepositoryService) CreateTag(ctx context.Context, path, name, commit, message string) error {
	return s.write(ctx, func() error { return git.CreateTag(ctx, path, name, commit, message) })
}

func (s *RepositoryService) DeleteTag(ctx context.Context, path, name string) error {
	return s.write(ctx, func() error { return git.DeleteTag(ctx, path, name) })
}

func (s *RepositoryService) PushTag(ctx context.Context, path, remote, name string) error {
	return s.write(ctx, func() error { return git.PushTag(ctx, path, remote, name) })
}

func (s *RepositoryService) CheckoutTag(ctx context.Context, path, name string) error {
	return s.write(ctx, func() error { return git.CheckoutTag(ctx, path, name) })
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

// Clone copies a repository from a URL or path into dir and opens it. The
// network operation runs outside the repository write lock: it touches only
// the new target directory.
func (*RepositoryService) Clone(ctx context.Context, address, dir, branch string) (git.Repository, error) {
	if err := git.Clone(ctx, address, dir, branch); err != nil {
		return git.Repository{}, err
	}
	return git.Open(ctx, dir)
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

func (s *RepositoryService) SaveTextFile(ctx context.Context, path, file, revision, content, lineEnding string) (string, error) {
	var updated string
	err := s.write(ctx, func() error {
		var err error
		updated, err = git.SaveTextFile(ctx, path, file, revision, content, lineEnding)
		return err
	})
	return updated, err
}

func (s *RepositoryService) AddToGitignore(ctx context.Context, path, file string) (bool, error) {
	var changed bool
	err := s.write(ctx, func() error {
		var err error
		changed, err = git.AddToGitignore(ctx, path, file)
		return err
	})
	return changed, err
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

func (s *RepositoryService) ApplyHistoryOperation(ctx context.Context, path, action string, targets []string, expected git.HeadState, mode string) error {
	return s.write(ctx, func() error { return git.ApplyHistoryOperation(ctx, path, action, targets, expected, mode) })
}

func (s *RepositoryService) FinishHistoryOperation(ctx context.Context, path, revision string, abort bool) error {
	return s.write(ctx, func() error { return git.FinishHistoryOperation(ctx, path, revision, abort) })
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
func (*RepositoryService) ReadFileRevision(ctx context.Context, repo, file, revision string, compareWorking bool) (git.FilePreview, error) {
	return discardCancelled(git.ReadFileRevision(ctx, repo, file, revision, compareWorking))
}

func (*RepositoryService) FileSelectionHistory(ctx context.Context, repo, file string, start, end int) (git.FileHistoryPage, error) {
	return discardCancelled(git.FileSelectionHistory(ctx, repo, file, start, end))
}

func (*RepositoryService) PrepareFileMutation(ctx context.Context, repo, file, action string) (string, error) {
	return discardCancelled(git.PrepareFileMutation(ctx, repo, file, action))
}

func (s *RepositoryService) ApplyFileMutation(ctx context.Context, repo, file, action, expected, message string) error {
	return s.write(ctx, func() error { return git.ApplyFileMutation(ctx, repo, file, action, expected, message) })
}

func (*RepositoryService) ProjectEntries(ctx context.Context, repo, folder string) ([]git.ProjectEntry, error) {
	return discardCancelled(git.ProjectEntries(repo, folder))
}

func (*RepositoryService) SearchProject(ctx context.Context, repo, folder, query string) ([]git.ProjectMatch, error) {
	return discardCancelled(git.SearchProject(ctx, repo, folder, query))
}

func (s *RepositoryService) ChangeProjectEntry(ctx context.Context, repo, action, source, revision, folder, name string) error {
	return s.write(ctx, func() error {
		return git.ChangeProjectEntry(ctx, repo, action, source, revision, folder, name)
	})
}

func (*RepositoryService) ProjectLocation(repo, path string) (string, error) {
	absolute, _, err := git.ProjectLocation(repo, path)
	return absolute, err
}

func (*RepositoryService) OpenProjectLocation(repo, path, action string) error {
	absolute, folder, err := git.ProjectLocation(repo, path)
	if err != nil {
		return err
	}
	return openProjectLocation(absolute, folder, action)
}
