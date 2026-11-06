package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeLifecycle(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "initial")
	commit(t, dir, "initial")
	base := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	target := filepath.Join(t.TempDir(), "linked 工作 tree")

	if err := AddWorktree(ctx, dir, target, "main", ""); err == nil {
		t.Fatal("checked out a branch already in use")
	}
	if err := AddWorktree(ctx, dir, target, "feature", base); err != nil {
		t.Fatal(err)
	}
	trees, err := Worktrees(ctx, dir)
	if err != nil || len(trees) != 2 || !trees[0].Main ||
		trees[1].Branch != "feature" || trees[1].Head != base {
		t.Fatalf("worktrees: %+v, %v", trees, err)
	}
	want, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.Stat(trees[1].Path)
	if err != nil || !os.SameFile(want, got) {
		t.Fatalf("listed path %q does not refer to %q: %v", trees[1].Path, target, err)
	}
	target = trees[1].Path
	fromLinked, err := Worktrees(ctx, target)
	if err != nil || len(fromLinked) != 2 || fromLinked[0] != trees[0] || fromLinked[1] != trees[1] {
		t.Fatalf("worktrees from linked checkout: %+v, %v", fromLinked, err)
	}
	if err := RemoveWorktree(ctx, dir, trees[0].Path); err == nil {
		t.Fatal("removed the main worktree")
	}
	if err := RemoveWorktree(ctx, target, target); err == nil {
		t.Fatal("removed the active worktree")
	}

	write(t, target, "file.txt", "local edit")
	if err := RemoveWorktree(ctx, dir, target); err == nil {
		t.Fatal("removed a worktree with local edits")
	}
	command(t, target, "restore", "file.txt")
	command(t, target, "config", "core.excludesFile", filepath.Join(dir, ".git", "info", "exclude"))
	write(t, dir, ".git/info/exclude", "ignored.txt\n")
	write(t, target, "ignored.txt", "local data")
	if err := RemoveWorktree(ctx, dir, target); err == nil {
		t.Fatal("removed ignored local data")
	}
	if err := os.Remove(filepath.Join(target, "ignored.txt")); err != nil {
		t.Fatal(err)
	}
	command(t, dir, "worktree", "lock", "--reason", "in use", target)
	locked, err := Worktrees(ctx, dir)
	if err != nil || len(locked) != 2 || !locked[1].Locked {
		t.Fatalf("locked worktree: %+v, %v", locked, err)
	}
	if err := RemoveWorktree(ctx, dir, target); err == nil {
		t.Fatal("removed a locked worktree")
	}
	command(t, dir, "worktree", "unlock", target)
	if err := RemoveWorktree(ctx, dir, target); err != nil {
		t.Fatal(err)
	}
	command(t, dir, "show-ref", "--verify", "refs/heads/feature")
	command(t, dir, "tag", "feature")
	if err := AddWorktree(ctx, dir, target, "feature", ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, target, "branch", "--show-current")); got != "feature" {
		t.Fatalf("expected feature checked out, got %q", got)
	}
}
