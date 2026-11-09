package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// syncFixture builds a bare remote, a local clone with main and feature
// tracking origin, and a peer clone used to publish remote changes.
func syncFixture(t *testing.T) (ctx context.Context, dir, peer, bare string) {
	t.Helper()
	ctx = context.Background()
	dir = repository(t)
	bare = filepath.Join(t.TempDir(), "remote.git")
	command(t, dir, "init", "--bare", "-b", "main", bare)
	command(t, dir, "remote", "add", "origin", bare)
	write(t, dir, "shared.txt", "base\n")
	commit(t, dir, "base")
	command(t, dir, "branch", "feature")
	command(t, dir, "push", "-u", "origin", "main", "feature")
	peer = repository(t)
	command(t, peer, "remote", "add", "origin", bare)
	command(t, peer, "fetch", "origin")
	command(t, peer, "checkout", "-B", "main", "origin/main")
	return ctx, dir, peer, bare
}

func publish(t *testing.T, peer, branch, file, content, title string) {
	t.Helper()
	command(t, peer, "checkout", "-B", branch, "origin/"+branch)
	write(t, peer, file, content)
	commit(t, peer, title)
	command(t, peer, "push", "origin", branch)
}

func rev(t *testing.T, dir, ref string) string {
	t.Helper()
	return strings.TrimSpace(command(t, dir, "rev-parse", ref))
}

func TestUpdateCurrentBranchFastForwardsAndKeepsLocalEdits(t *testing.T) {
	ctx, dir, peer, _ := syncFixture(t)
	publish(t, peer, "main", "peer.txt", "peer\n", "peer update")
	write(t, dir, "shared.txt", "unsaved local edit\n")

	result, err := UpdateBranch(ctx, dir, "main", UpdateMerge)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Current || result.Outcome != UpdateFastForward || result.Upstream != "origin/main" || result.After != rev(t, peer, "HEAD") {
		t.Fatalf("unexpected result %+v", result)
	}
	if preview, err := WorkingPreview(dir, "shared.txt"); err != nil || preview.Content != "unsaved local edit\n" {
		t.Fatalf("update lost the working edit: %+v, %v", preview, err)
	}
	if again, err := UpdateBranch(ctx, dir, "main", UpdateMerge); err != nil || again.Outcome != UpdateUpToDate {
		t.Fatalf("second update: %+v, %v", again, err)
	}
}

func TestUpdateCurrentBranchMergeOrRebaseWhenDiverged(t *testing.T) {
	for _, method := range []string{UpdateMerge, UpdateRebase} {
		t.Run(method, func(t *testing.T) {
			ctx, dir, peer, _ := syncFixture(t)
			publish(t, peer, "main", "peer.txt", "peer\n", "peer update")
			write(t, dir, "local.txt", "local\n")
			commit(t, dir, "local update")

			result, err := UpdateBranch(ctx, dir, "main", method)
			if err != nil {
				t.Fatal(err)
			}
			parents := strings.Fields(command(t, dir, "log", "-1", "--format=%P"))
			switch method {
			case UpdateMerge:
				if result.Outcome != UpdateMerged || len(parents) != 2 {
					t.Fatalf("merge update: %+v parents=%d", result, len(parents))
				}
			default:
				log := strings.TrimSpace(command(t, dir, "log", "--format=%s"))
				if result.Outcome != UpdateRebased || len(parents) != 1 || !strings.HasPrefix(log, "local update\npeer update") {
					t.Fatalf("rebase update: %+v log=%q", result, log)
				}
			}
		})
	}
}

func TestUpdateNonCurrentBranchOnlyMovesThatBranch(t *testing.T) {
	ctx, dir, peer, _ := syncFixture(t)
	publish(t, peer, "feature", "feature.txt", "feature\n", "feature update")
	head := rev(t, dir, "HEAD")
	write(t, dir, "shared.txt", "dirty edit\n")

	result, err := UpdateBranch(ctx, dir, "feature", UpdateRebase)
	if err != nil {
		t.Fatal(err)
	}
	if result.Current || result.Outcome != UpdateFastForward || rev(t, dir, "refs/heads/feature") != rev(t, peer, "HEAD") {
		t.Fatalf("feature was not fast-forwarded: %+v", result)
	}
	if rev(t, dir, "HEAD") != head || strings.TrimSpace(command(t, dir, "symbolic-ref", "HEAD")) != "refs/heads/main" {
		t.Fatal("updating a non-current branch moved HEAD")
	}
	if preview, err := WorkingPreview(dir, "shared.txt"); err != nil || preview.Content != "dirty edit\n" {
		t.Fatalf("working tree changed: %+v, %v", preview, err)
	}

	// A branch that is only ahead of its upstream is left untouched.
	command(t, dir, "checkout", "-q", "feature")
	command(t, dir, "commit", "--allow-empty", "-q", "-m", "ahead on feature")
	command(t, dir, "checkout", "-q", "main")
	ahead := rev(t, dir, "refs/heads/feature")
	if result, err := UpdateBranch(ctx, dir, "feature", UpdateMerge); err != nil || result.Outcome != UpdateUpToDate || rev(t, dir, "refs/heads/feature") != ahead {
		t.Fatalf("ahead branch: %+v, %v", result, err)
	}

	// Divergence is refused for a branch that is not checked out.
	command(t, dir, "commit", "--allow-empty", "-m", "noop on main")
	command(t, dir, "update-ref", "refs/heads/feature", rev(t, dir, "HEAD"), rev(t, dir, "refs/heads/feature"))
	if _, err := UpdateBranch(ctx, dir, "feature", UpdateMerge); err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("diverged non-current branch: %v", err)
	}
}

func TestUpdateNonCurrentBranchRefusesDivergenceAndKeepsRef(t *testing.T) {
	ctx, dir, peer, _ := syncFixture(t)
	publish(t, peer, "feature", "feature.txt", "remote\n", "remote feature")
	command(t, dir, "checkout", "-q", "feature")
	write(t, dir, "feature.txt", "local\n")
	commit(t, dir, "local feature")
	command(t, dir, "checkout", "-q", "main")
	before := rev(t, dir, "refs/heads/feature")

	if _, err := UpdateBranch(ctx, dir, "feature", UpdateMerge); err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("expected divergence error, got %v", err)
	}
	if rev(t, dir, "refs/heads/feature") != before {
		t.Fatal("a refused update moved the branch")
	}
	// The remote-tracking ref still reflects the fetch, as in IDEA.
	if rev(t, dir, "refs/remotes/origin/feature") != rev(t, peer, "HEAD") {
		t.Fatal("update did not fetch the upstream")
	}
}

func TestUpdateBranchPreconditions(t *testing.T) {
	ctx, dir, _, _ := syncFixture(t)
	command(t, dir, "branch", "local-only")
	if _, err := UpdateBranch(ctx, dir, "local-only", UpdateMerge); err == nil || !strings.Contains(err.Error(), "no tracked branch") {
		t.Fatalf("branch without upstream: %v", err)
	}
	if _, err := UpdateBranch(ctx, dir, "missing", UpdateMerge); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("missing branch: %v", err)
	}
	if _, err := UpdateBranch(ctx, dir, "--all", UpdateMerge); err == nil {
		t.Fatal("accepted an option-like branch")
	}
	if _, err := UpdateBranch(ctx, dir, "main", "reset"); err == nil {
		t.Fatal("accepted an unknown update method")
	}
	linked := filepath.Join(t.TempDir(), "linked")
	command(t, dir, "worktree", "add", "-q", linked, "feature")
	if _, err := UpdateBranch(ctx, dir, "feature", UpdateMerge); err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("branch checked out elsewhere: %v", err)
	}
	command(t, dir, "checkout", "-q", "--detach")
	if _, err := Pull(ctx, dir, PullRequest{Branch: "main", Remote: "origin", RemoteBranch: "main", Mode: PullMerge}); err == nil ||
		!strings.Contains(err.Error(), "detached") {
		t.Fatalf("pull on detached HEAD: %v", err)
	}
}

func TestUpdateConflictLeavesRecoverableMerge(t *testing.T) {
	ctx, dir, peer, _ := syncFixture(t)
	publish(t, peer, "main", "shared.txt", "remote\n", "remote change")
	write(t, dir, "shared.txt", "local\n")
	commit(t, dir, "local change")

	_, err := UpdateBranch(ctx, dir, "main", UpdateMerge)
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected a conflict, got %v", err)
	}
	if checkOperationState(ctx, dir) == nil {
		t.Fatal("conflict did not leave the merge in progress")
	}
	if _, err := UpdateBranch(ctx, dir, "main", UpdateMerge); err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Fatalf("update during merge: %v", err)
	}
	command(t, dir, "merge", "--abort")
}

func TestUpdateUnreachableRemoteKeepsHead(t *testing.T) {
	ctx, dir, _, bare := syncFixture(t)
	head := rev(t, dir, "HEAD")
	if err := os.RemoveAll(bare); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateBranch(ctx, dir, "main", UpdateMerge); err == nil {
		t.Fatal("update succeeded without a remote")
	}
	if rev(t, dir, "HEAD") != head {
		t.Fatal("a failed fetch moved HEAD")
	}
}

func TestPullSelectedRemoteBranchIntoCurrent(t *testing.T) {
	ctx, dir, peer, _ := syncFixture(t)
	publish(t, peer, "feature", "feature.txt", "feature\n", "feature work")
	command(t, dir, "fetch", "origin")
	write(t, dir, "local.txt", "local\n")
	commit(t, dir, "local main")

	if _, err := Pull(ctx, dir, PullRequest{Branch: "main", Remote: "origin", RemoteBranch: "feature", Mode: PullFastForward}); err == nil {
		t.Fatal("fast-forward-only pull accepted diverged history")
	}
	if _, err := Pull(ctx, dir, PullRequest{Branch: "main", Remote: "origin", RemoteBranch: "feature", Mode: PullRebase, NoCommit: true}); err == nil {
		t.Fatal("accepted --no-commit with rebase")
	}
	if _, err := Pull(ctx, dir, PullRequest{Branch: "main", Remote: "origin", RemoteBranch: "nope", Mode: PullMerge}); err == nil {
		t.Fatal("pulled an unknown remote branch")
	}
	result, err := Pull(ctx, dir, PullRequest{Branch: "main", Remote: "origin", RemoteBranch: "feature", Mode: PullMerge})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != UpdateMerged || result.Upstream != "origin/feature" {
		t.Fatalf("pull result %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "feature.txt")); err != nil {
		t.Fatal("pulled content is missing")
	}
	// Pulling another branch does not re-point the upstream.
	if upstream := strings.TrimSpace(command(t, dir, "rev-parse", "--abbrev-ref", "main@{upstream}")); upstream != "origin/main" {
		t.Fatalf("pull changed upstream to %s", upstream)
	}
}

func TestPushNonCurrentBranchUsesItsOwnUpstream(t *testing.T) {
	ctx, dir, _, bare := syncFixture(t)
	command(t, dir, "checkout", "-q", "feature")
	write(t, dir, "feature.txt", "feature\n")
	commit(t, dir, "feature commit")
	command(t, dir, "checkout", "-q", "main")
	if err := PushBranch(ctx, dir, "feature", "origin", "main", PushOptions{}); err == nil {
		t.Fatal("pushed feature to a target that is not its upstream")
	}
	if err := PushBranch(ctx, dir, "feature", "origin", "feature", PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if rev(t, bare, "refs/heads/feature") != rev(t, dir, "refs/heads/feature") {
		t.Fatal("feature was not pushed")
	}
	if rev(t, bare, "refs/heads/main") == rev(t, dir, "refs/heads/feature") {
		t.Fatal("push touched main")
	}
}
