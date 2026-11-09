package git

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateBranchAtSelectedCommit(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "first")
	commit(t, dir, "first")
	base := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	write(t, dir, "file.txt", "second")
	commit(t, dir, "second")

	if err := CreateBranch(ctx, dir, "feature/新分支", base); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "rev-parse", "refs/heads/feature/新分支")); got != base {
		t.Fatalf("branch points at %q, expected %q", got, base)
	}
	if got := strings.TrimSpace(command(t, dir, "branch", "--show-current")); got != "main" {
		t.Fatalf("creating branch changed HEAD to %q", got)
	}
	if err := CreateBranch(ctx, dir, "main", base); err == nil {
		t.Fatal("overwrote existing branch")
	}
	if err := CreateBranch(ctx, dir, "-f", base); err == nil {
		t.Fatal("accepted a branch name beginning with a flag")
	}
}

func TestSwitchBranchProtectsLocalEdits(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "main")
	commit(t, dir, "initial")
	command(t, dir, "switch", "-c", "feature")
	write(t, dir, "file.txt", "feature")
	commit(t, dir, "feature")
	command(t, dir, "switch", "main")

	write(t, dir, "file.txt", "local edit")
	if err := SwitchBranch(ctx, dir, "feature"); err == nil {
		t.Fatal("switched with an overlapping edit")
	}
	preview, err := WorkingPreview(dir, "file.txt")
	if err != nil || preview.Content != "local edit" {
		t.Fatalf("local edit lost: %+v, %v", preview, err)
	}
	write(t, dir, "file.txt", "main")
	if err := SwitchBranch(ctx, dir, "feature"); err != nil {
		t.Fatal(err)
	}
	repo, err := Open(ctx, dir)
	if err != nil || repo.Head != "feature" {
		t.Fatalf("active branch: %+v, %v", repo, err)
	}
}

func TestRenameAndDeleteMergedBranch(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "initial")
	commit(t, dir, "initial")
	command(t, dir, "branch", "old-name")
	if err := RenameBranch(ctx, dir, "old-name", "main"); err == nil {
		t.Fatal("renamed over an existing branch")
	}
	if err := RenameBranch(ctx, dir, "old-name", "feature/新名称"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "branch", "--list", "old-name", "feature/新名称", "--format=%(refname:short)")); got != "feature/新名称" {
		t.Fatalf("renamed refs: %q", got)
	}
	if _, err := InspectBranchDeletion(ctx, dir, "main"); err == nil {
		t.Fatal("offered to delete the current branch")
	}
	info, err := InspectBranchDeletion(ctx, dir, "feature/新名称")
	if err != nil || info.Unmerged != 0 {
		t.Fatalf("inspection: %+v, %v", info, err)
	}
	if err := DeleteBranch(ctx, dir, "main", info.Tip, true); err == nil {
		t.Fatal("deleted current branch")
	}
	if err := DeleteBranch(ctx, dir, "feature/新名称", info.Tip, false); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "branch", "--list", "feature/新名称")); got != "" {
		t.Fatalf("deleted branch still present: %q", got)
	}
}

func TestDeleteBranchRejectsCommitsOutsideHEAD(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "main")
	commit(t, dir, "main")
	command(t, dir, "switch", "-c", "feature")
	write(t, dir, "file.txt", "feature")
	commit(t, dir, "feature")
	command(t, dir, "branch", "same-tip")
	command(t, dir, "branch", "--set-upstream-to=same-tip", "feature")
	command(t, dir, "switch", "main")

	info, err := InspectBranchDeletion(ctx, dir, "feature")
	if err != nil || info.Unmerged != 1 || info.Base != "main" {
		t.Fatalf("inspection: %+v, %v", info, err)
	}
	if err := DeleteBranch(ctx, dir, "feature", info.Tip, false); err == nil {
		t.Fatal("deleted a branch merged only into its upstream, not HEAD")
	}
	if got := strings.TrimSpace(command(t, dir, "log", "-1", "--format=%s", "feature")); got != "feature" {
		t.Fatalf("feature tip changed: %q", got)
	}

	// A branch that moved after the review is kept even with force.
	command(t, dir, "branch", "-f", "feature", "same-tip~1")
	if err := DeleteBranch(ctx, dir, "feature", info.Tip, true); err == nil || !strings.Contains(err.Error(), "moved") {
		t.Fatalf("stale tip: %v", err)
	}
	command(t, dir, "branch", "-f", "feature", info.Tip)
	linked := filepath.Join(t.TempDir(), "linked")
	command(t, dir, "worktree", "add", "-q", linked, "feature")
	if err := DeleteBranch(ctx, dir, "feature", info.Tip, true); err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("branch in another worktree: %v", err)
	}
	command(t, dir, "worktree", "remove", "--force", linked)

	// Force deletes the reviewed unmerged branch and drops its tracking config.
	if err := DeleteBranch(ctx, dir, "feature", info.Tip, true); err != nil {
		t.Fatal(err)
	}
	if out := strings.TrimSpace(command(t, dir, "branch", "--list", "feature")); out != "" {
		t.Fatalf("feature still present: %q", out)
	}
	if err := CreateBranch(ctx, dir, "feature", info.Tip); err != nil {
		t.Fatalf("restore: %v", err)
	}
}
