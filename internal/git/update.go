package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Update methods mirror IDEA's "Update Type" setting (GitVcsSettings.updateMethod).
const (
	UpdateMerge  = "merge"
	UpdateRebase = "rebase"
)

// Update outcomes reported to the UI.
const (
	UpdateUpToDate    = "up-to-date"
	UpdateFastForward = "fast-forward"
	UpdateMerged      = "merged"
	UpdateRebased     = "rebased"
)

// BranchTracking describes one local branch and its configured upstream.
type BranchTracking struct {
	Branch       string `json:"branch"`
	Remote       string `json:"remote"`
	RemoteBranch string `json:"remoteBranch"`
	TrackingRef  string `json:"trackingRef"`
}

func (t BranchTracking) upstreamLabel() string { return t.Remote + "/" + t.RemoteBranch }

type UpdateResult struct {
	Branch   string `json:"branch"`
	Upstream string `json:"upstream"`
	Current  bool   `json:"current"`
	Outcome  string `json:"outcome"`
	Before   string `json:"before"`
	After    string `json:"after"`
}

// branchTracking reads the upstream of an explicit local branch. It never
// falls back to HEAD, so callers act on the branch the user selected.
func branchTracking(ctx context.Context, dir, branch string) (BranchTracking, error) {
	if err := checkBranchName(ctx, dir, branch); err != nil {
		return BranchTracking{}, err
	}
	ref := "refs/heads/" + branch
	data, err := run(ctx, dir, nil, 1<<20, "for-each-ref",
		"--format=%(refname)%00%(upstream:remotename)%00%(upstream:remoteref)%00%(upstream)", "--", ref)
	if err != nil {
		return BranchTracking{}, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(strings.TrimSuffix(line, "\r"), "\x00")
		if len(fields) != 4 || fields[0] != ref {
			continue
		}
		tracking := BranchTracking{Branch: branch, Remote: fields[1], TrackingRef: fields[3]}
		if strings.HasPrefix(fields[2], "refs/heads/") {
			tracking.RemoteBranch = strings.TrimPrefix(fields[2], "refs/heads/")
		}
		return tracking, nil
	}
	return BranchTracking{}, fmt.Errorf("local branch %s no longer exists; refresh the branch list", branch)
}

func remoteNames(ctx context.Context, dir string) ([]string, error) {
	data, err := run(ctx, dir, nil, 1<<20, "remote")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(data)), nil
}

func requireRemoteUpstream(ctx context.Context, dir string, tracking BranchTracking) error {
	remotes, err := remoteNames(ctx, dir)
	if err != nil {
		return err
	}
	if tracking.Remote == "" || tracking.RemoteBranch == "" || !slices.Contains(remotes, tracking.Remote) ||
		!strings.HasPrefix(tracking.TrackingRef, "refs/remotes/") {
		return fmt.Errorf("no tracked branch configured for branch %s; push it with an upstream or set one first", tracking.Branch)
	}
	return nil
}

func revision(ctx context.Context, dir, ref string) (string, error) {
	data, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s does not resolve to a commit", strings.TrimPrefix(ref, "refs/"))
	}
	id := strings.TrimSpace(string(data))
	if !objectID(id) {
		return "", errors.New("Git returned an invalid object ID")
	}
	return id, nil
}

// isAncestor reports whether ancestor is reachable from descendant.
func isAncestor(ctx context.Context, dir, ancestor, descendant string) (bool, error) {
	_, err := run(ctx, dir, nil, 4096, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// checkedOutElsewhere returns the worktree path that has branch checked out,
// excluding dir itself.
func checkedOutElsewhere(ctx context.Context, dir, branch string) (string, error) {
	trees, err := Worktrees(ctx, dir)
	if err != nil {
		return "", err
	}
	top, err := metadataPath(ctx, dir, "--show-toplevel")
	if err != nil {
		return "", err
	}
	for _, tree := range trees {
		if tree.Branch == branch && !samePath(tree.Path, top) {
			return tree.Path, nil
		}
	}
	return "", nil
}

func fetchTracking(ctx context.Context, dir string, tracking BranchTracking) error {
	// An explicit refspec updates the remote-tracking ref even when other Git
	// clients rewrite FETCH_HEAD, and leaves every other ref alone.
	refspec := "+refs/heads/" + tracking.RemoteBranch + ":" + tracking.TrackingRef
	return runRemote(ctx, dir, "fetch", "--no-write-fetch-head", "--no-tags", "--no-prune", "--refmap=",
		"--no-recurse-submodules", "--", tracking.Remote, refspec)
}

// UpdateBranch follows IDEA's branch "Update" action (GitUpdateSelectedBranchAction):
// the checked-out branch is fetched and merged or rebased onto its upstream,
// preserving local changes; any other local branch is only fast-forwarded to
// its upstream and never touches HEAD or the working tree.
func UpdateBranch(ctx context.Context, dir, branch, method string) (UpdateResult, error) {
	if method != UpdateMerge && method != UpdateRebase {
		return UpdateResult{}, errors.New("select a supported update method: merge or rebase")
	}
	tracking, err := branchTracking(ctx, dir, branch)
	if err != nil {
		return UpdateResult{}, err
	}
	if err := requireRemoteUpstream(ctx, dir, tracking); err != nil {
		return UpdateResult{}, err
	}
	headRef, err := headReference(ctx, dir)
	if err != nil {
		return UpdateResult{}, err
	}
	result := UpdateResult{Branch: branch, Upstream: tracking.upstreamLabel(), Current: headRef == "refs/heads/"+branch}
	if result.Current {
		return updateCurrent(ctx, dir, tracking, method, result)
	}
	return updateOther(ctx, dir, tracking, result)
}

func updateOther(ctx context.Context, dir string, tracking BranchTracking, result UpdateResult) (UpdateResult, error) {
	ref := "refs/heads/" + tracking.Branch
	if path, err := checkedOutElsewhere(ctx, dir, tracking.Branch); err != nil {
		return result, err
	} else if path != "" {
		return result, fmt.Errorf("branch %s is checked out in worktree %s; update it from that worktree", tracking.Branch, path)
	}
	before, err := revision(ctx, dir, ref)
	if err != nil {
		return result, err
	}
	result.Before, result.After = before, before
	if err := fetchTracking(ctx, dir, tracking); err != nil {
		return result, err
	}
	remote, err := revision(ctx, dir, tracking.TrackingRef)
	if err != nil {
		return result, err
	}
	if upToDate, err := isAncestor(ctx, dir, remote, before); err != nil {
		return result, err
	} else if upToDate {
		result.Outcome = UpdateUpToDate
		return result, nil
	}
	if ff, err := isAncestor(ctx, dir, before, remote); err != nil {
		return result, err
	} else if !ff {
		return result, fmt.Errorf("branch %s has diverged from %s and cannot be fast-forwarded; check it out and update it to merge or rebase",
			tracking.Branch, tracking.upstreamLabel())
	}
	headRef, err := headReference(ctx, dir)
	if err != nil {
		return result, err
	}
	if headRef == ref {
		return result, errors.New("the branch was checked out during fetch; refresh and update it again")
	}
	// Compare-and-swap: refuse when another client moved the branch meanwhile.
	if _, err := run(ctx, dir, nil, 4096, "update-ref", "-m", "update: fast-forward to "+tracking.upstreamLabel(),
		ref, remote, before); err != nil {
		return result, errors.New("the branch changed during update; refresh and try again")
	}
	result.After, result.Outcome = remote, UpdateFastForward
	return result, nil
}

func updateCurrent(ctx context.Context, dir string, tracking BranchTracking, method string, result UpdateResult) (UpdateResult, error) {
	if err := checkOperationState(ctx, dir); err != nil {
		return result, err
	}
	before, err := headID(ctx, dir)
	if err != nil {
		return result, err
	}
	result.Before, result.After = before, before
	if err := fetchTracking(ctx, dir, tracking); err != nil {
		return result, err
	}
	if err := requireSameHead(ctx, dir, "refs/heads/"+tracking.Branch, before); err != nil {
		return result, err
	}
	remote, err := revision(ctx, dir, tracking.TrackingRef)
	if err != nil {
		return result, err
	}
	if upToDate, err := isAncestor(ctx, dir, remote, before); err != nil {
		return result, err
	} else if upToDate {
		result.Outcome = UpdateUpToDate
		return result, nil
	}
	ff, err := isAncestor(ctx, dir, before, remote)
	if err != nil {
		return result, err
	}
	switch {
	case ff:
		result.Outcome = UpdateFastForward
		_, err = run(ctx, dir, nil, 1<<20, "-c", "submodule.recurse=false", "merge", "--ff-only", "--autostash",
			"--no-overwrite-ignore", "--", tracking.TrackingRef)
	case method == UpdateMerge:
		result.Outcome = UpdateMerged
		_, err = run(ctx, dir, nil, 1<<20, "-c", "submodule.recurse=false", "merge", "--no-edit", "--no-squash", "--commit",
			"--autostash", "--no-overwrite-ignore", "--", tracking.TrackingRef)
	default:
		result.Outcome = UpdateRebased
		_, err = run(ctx, dir, nil, 1<<20, "-c", "submodule.recurse=false", "rebase", "--autostash", "--", tracking.TrackingRef)
	}
	if after, headErr := headID(ctx, dir); headErr == nil {
		result.After = after
	}
	if err != nil {
		return result, integrationFailure(ctx, dir, err)
	}
	return result, nil
}

func requireSameHead(ctx context.Context, dir, ref, head string) error {
	currentRef, err := headReference(ctx, dir)
	if err != nil {
		return err
	}
	currentHead, err := headID(ctx, dir)
	if err != nil {
		return err
	}
	if currentRef != ref || currentHead != head {
		return errors.New("HEAD changed during fetch; review the repository before trying again")
	}
	return checkOperationState(ctx, dir)
}

// integrationFailure keeps the Git state for the conflict console and tells
// the user what to do next instead of reporting a bare exit status.
func integrationFailure(ctx context.Context, dir string, err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	if stateErr := checkOperationState(ctx, dir); stateErr != nil {
		return fmt.Errorf("the update stopped with conflicts; resolve them, then continue or abort the operation: %v", err)
	}
	return remoteFailure(err)
}

// Pull options follow IDEA's Pull dialog (GitPullOption).
const (
	PullFastForward = "ff"
	PullMerge       = "merge"
	PullNoFF        = "no-ff"
	PullSquash      = "squash"
	PullRebase      = "rebase"
)

type PullRequest struct {
	// Branch is the checked-out local branch the user reviewed in the dialog.
	Branch       string `json:"branch"`
	Remote       string `json:"remote"`
	RemoteBranch string `json:"remoteBranch"`
	Mode         string `json:"mode"`
	NoCommit     bool   `json:"noCommit"`
	NoVerify     bool   `json:"noVerify"`
	Autostash    bool   `json:"autostash"`
}

// Pull fetches an explicitly selected remote branch and integrates it into
// the checked-out branch. Unlike UpdateBranch the source is chosen by the
// user and does not have to be the configured upstream.
func Pull(ctx context.Context, dir string, request PullRequest) (UpdateResult, error) {
	result := UpdateResult{Branch: request.Branch, Current: true}
	if request.Mode == PullRebase && request.NoCommit {
		return result, errors.New("--no-commit cannot be combined with rebase")
	}
	switch request.Mode {
	case PullFastForward, PullMerge, PullNoFF, PullSquash, PullRebase:
	default:
		return result, errors.New("select a supported pull mode: fast-forward, merge, no fast-forward, squash, or rebase")
	}
	if err := checkBranchName(ctx, dir, request.Branch); err != nil {
		return result, err
	}
	headRef, err := headReference(ctx, dir)
	if err != nil {
		return result, err
	}
	if headRef == "" {
		return result, errors.New("pull needs a checked-out local branch; HEAD is detached")
	}
	ref := "refs/heads/" + request.Branch
	if headRef != ref {
		return result, errors.New("the current branch changed; review the pull target")
	}
	remotes, err := remoteNames(ctx, dir)
	if err != nil {
		return result, err
	}
	if !slices.Contains(remotes, request.Remote) {
		return result, errors.New("select a configured Git remote")
	}
	if request.RemoteBranch == "" || strings.HasPrefix(request.RemoteBranch, "-") ||
		checkBranchName(ctx, dir, request.RemoteBranch) != nil {
		return result, errors.New("select a remote branch to pull")
	}
	tracking := BranchTracking{Branch: request.Branch, Remote: request.Remote, RemoteBranch: request.RemoteBranch,
		TrackingRef: "refs/remotes/" + request.Remote + "/" + request.RemoteBranch}
	result.Upstream = tracking.upstreamLabel()
	// Match IDEA: the dialog offers remote branches this clone already knows.
	if _, err := revision(ctx, dir, tracking.TrackingRef); err != nil {
		return result, fmt.Errorf("%s is not a known remote branch; fetch the remote first", tracking.upstreamLabel())
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return result, err
	}
	before, err := headID(ctx, dir)
	if err != nil {
		return result, err
	}
	result.Before, result.After = before, before
	if !request.Autostash {
		if err := requireCleanWorktree(ctx, dir); err != nil {
			return result, err
		}
	}
	if err := fetchTracking(ctx, dir, tracking); err != nil {
		return result, err
	}
	if err := requireSameHead(ctx, dir, ref, before); err != nil {
		return result, err
	}
	if !request.Autostash {
		if err := requireCleanWorktree(ctx, dir); err != nil {
			return result, err
		}
	}
	autostash := "--no-autostash"
	if request.Autostash {
		autostash = "--autostash"
	}
	var args []string
	if request.Mode == PullRebase {
		result.Outcome = UpdateRebased
		args = []string{"-c", "submodule.recurse=false", "rebase", autostash}
		if request.NoVerify {
			args = append(args, "--no-verify")
		}
	} else {
		result.Outcome = UpdateMerged
		args = []string{"-c", "submodule.recurse=false", "merge", "--no-edit", autostash, "--no-overwrite-ignore"}
		switch request.Mode {
		case PullFastForward:
			args = append(args, "--ff-only")
		case PullNoFF:
			args = append(args, "--no-ff")
		case PullSquash:
			args = append(args, "--squash")
		}
		if request.NoCommit {
			args = append(args, "--no-commit")
		}
		if request.NoVerify {
			args = append(args, "--no-verify")
		}
	}
	_, err = run(ctx, dir, nil, 1<<20, append(args, "--", tracking.TrackingRef)...)
	if after, headErr := headID(ctx, dir); headErr == nil {
		result.After = after
	}
	if err != nil {
		return result, integrationFailure(ctx, dir, err)
	}
	if result.After == before {
		result.Outcome = UpdateUpToDate
		if request.Mode == PullSquash || request.NoCommit {
			result.Outcome = UpdateMerged
		}
	} else if ff, _ := isAncestor(ctx, dir, before, result.After); ff && request.Mode != PullRebase {
		if parents, _ := run(ctx, dir, nil, 4096, "rev-list", "--parents", "-n", "1", result.After); len(strings.Fields(string(parents))) < 3 {
			result.Outcome = UpdateFastForward
		}
	}
	return result, nil
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(filepath.FromSlash(a)), filepath.Clean(filepath.FromSlash(b))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// ReadBranchTracking returns the upstream of an explicit local branch for the
// push and update dialogs. A branch without upstream returns empty fields.
func ReadBranchTracking(ctx context.Context, dir, branch string) (BranchTracking, error) {
	return branchTracking(ctx, dir, branch)
}
