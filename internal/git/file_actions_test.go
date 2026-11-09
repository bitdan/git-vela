package git

import (
	"context"
	"strings"
	"testing"
)

func TestFileMutationChecksReviewedStateAndKeepsOtherStagedFiles(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "one.txt", "first\nsecond\n")
	write(t, dir, "two.txt", "base\n")
	commit(t, dir, "base")
	write(t, dir, "one.txt", "reviewed\nsecond\n")
	write(t, dir, "two.txt", "staged separately\n")
	command(t, dir, "add", "two.txt")

	token, err := PrepareFileMutation(ctx, dir, "one.txt", "rollback")
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "one.txt", "changed after review\nsecond\n")
	if err := ApplyFileMutation(ctx, dir, "one.txt", "rollback", token, ""); err == nil {
		t.Fatal("accepted a stale rollback confirmation")
	}
	if got := read(t, dir, "one.txt"); !strings.Contains(got, "changed after review") {
		t.Fatal("stale rollback changed the working file")
	}

	token, err = PrepareFileMutation(ctx, dir, "one.txt", "commit")
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyFileMutation(ctx, dir, "one.txt", "commit", token, "one only"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")); got != "one.txt" {
		t.Fatalf("commit includes %q", got)
	}
	if got := strings.TrimSpace(command(t, dir, "diff", "--cached", "--name-only")); got != "two.txt" {
		t.Fatalf("remaining staged files: %q", got)
	}
	write(t, dir, "one.txt", "discard me\n")
	command(t, dir, "add", "one.txt")
	token, err = PrepareFileMutation(ctx, dir, "one.txt", "rollback")
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyFileMutation(ctx, dir, "one.txt", "rollback", token, ""); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "one.txt"); !strings.Contains(got, "changed after review") {
		t.Fatalf("rollback restored %q", got)
	}
	if got := strings.TrimSpace(command(t, dir, "diff", "--cached", "--name-only")); got != "two.txt" {
		t.Fatalf("rollback affected other staged files: %q", got)
	}
}

func TestReadFileRevisionAndSelectionHistory(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "one.txt", "first\nsecond\n")
	commit(t, dir, "base")
	write(t, dir, "one.txt", "changed\nsecond\n")
	commit(t, dir, "first line changed")
	write(t, dir, "one.txt", "changed\nsecond line updated\n")
	commit(t, dir, "second line changed")
	page, err := FileSelectionHistory(ctx, dir, "one.txt", 1, 1)
	if err != nil || len(page.Commits) != 2 || page.Commits[0].Subject != "first line changed" {
		t.Fatalf("selection history: %+v, %v", page, err)
	}
	got, err := ReadFileRevision(ctx, dir, "one.txt", "HEAD~2", false)
	if err != nil || got.Content != "first\nsecond\n" {
		t.Fatalf("old file: %+v, %v", got, err)
	}
	got, err = ReadFileRevision(ctx, dir, "one.txt", "HEAD~2", true)
	if err != nil || !strings.Contains(got.Content, "+second line updated") {
		t.Fatalf("working comparison: %+v, %v", got, err)
	}
}
