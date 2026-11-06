package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkingChangesAndPreview(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "tracked.txt", "original")
	commit(t, dir, "initial")
	write(t, dir, "tracked.txt", "staged")
	command(t, dir, "add", "tracked.txt")
	write(t, dir, "tracked.txt", "working")
	write(t, dir, "新 file.txt", "untracked")
	files, err := WorkingChanges(ctx, dir)
	if err != nil || len(files) != 2 || files[0].Path != "tracked.txt" || files[0].Status != "MM" || files[1].Path != "新 file.txt" || files[1].Status != "??" {
		t.Fatalf("working status: %+v, %v", files, err)
	}
	preview, err := WorkingPreview(dir, files[0].Path)
	if err != nil || preview.Kind != "text" || preview.Content != "working" {
		t.Fatalf("working preview: %+v, %v", preview, err)
	}
	for _, path := range []string{"../outside.txt", ".git/config", "C:/Windows/win.ini"} {
		if _, err := WorkingPreview(dir, path); err == nil {
			t.Fatalf("accepted unsafe path %q", path)
		}
	}
}

func TestStageAndUnstageOneFile(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	target := "新 [file].txt"
	write(t, dir, target, "original")
	write(t, dir, "other.txt", "original")
	commit(t, dir, "initial")
	write(t, dir, target, "changed")
	write(t, dir, "other.txt", "other change")

	if err := SetStaged(ctx, dir, target, true); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "diff", "--cached", "--name-only", "-z"); got != target+"\x00" {
		t.Fatalf("staged paths = %q", got)
	}
	write(t, dir, target, "later edit")
	if err := SetStaged(ctx, dir, target, false); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("staged paths after unstage = %q", got)
	}
	preview, err := WorkingPreview(dir, target)
	if err != nil || preview.Content != "later edit" {
		t.Fatalf("unstage changed the working file: %+v, %v", preview, err)
	}
	if err := SetStaged(ctx, dir, ".", true); err == nil {
		t.Fatal("accepted whole-repository path")
	}
}

func TestUnstageBeforeFirstCommit(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "new.txt", "keep")
	for _, staged := range []bool{true, false} {
		if err := SetStaged(ctx, dir, "new.txt", staged); err != nil {
			t.Fatal(err)
		}
	}
	files, err := WorkingChanges(ctx, dir)
	if err != nil || len(files) != 1 || files[0].Status != "??" {
		t.Fatalf("unborn status: %+v, %v", files, err)
	}
}

func TestUnstageRenameKeepsWorkingPaths(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "old.txt", "keep")
	commit(t, dir, "initial")
	command(t, dir, "mv", "--", "old.txt", "new.txt")
	if err := SetStaged(ctx, dir, "new.txt", false); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("rename still staged: %q", got)
	}
	old, oldErr := WorkingPreview(dir, "old.txt")
	newFile, newErr := WorkingPreview(dir, "new.txt")
	if oldErr != nil || newErr != nil || old.Kind != "missing" || newFile.Content != "keep" {
		t.Fatalf("unstage modified working paths: %+v (%v), %+v (%v)", old, oldErr, newFile, newErr)
	}
}

func TestWorkingDiffAndCommitUseTheIndex(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	file := "tracked [1].txt"
	write(t, dir, file, "staged text\n")
	if err := SetStaged(ctx, dir, file, true); err != nil {
		t.Fatal(err)
	}
	write(t, dir, file, "later edit\n")
	staged, err := WorkingDiff(ctx, dir, file, true)
	if err != nil || !strings.Contains(staged.Content, "+staged text") || strings.Contains(staged.Content, "later edit") {
		t.Fatalf("staged patch = %+v, %v", staged, err)
	}
	unstaged, err := WorkingDiff(ctx, dir, file, false)
	if err != nil || !strings.Contains(unstaged.Content, "-staged text") || !strings.Contains(unstaged.Content, "+later edit") {
		t.Fatalf("unstaged patch = %+v, %v", unstaged, err)
	}
	if err := CommitIndex(ctx, dir, " \n "); err == nil {
		t.Fatal("committed with a blank message")
	}
	if err := CommitIndex(ctx, dir, "First change\n\nMessage body"); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "show", "HEAD:"+file); got != "staged text\n" {
		t.Fatalf("committed content = %q", got)
	}
	if got := command(t, dir, "log", "-1", "--format=%B"); strings.TrimSpace(got) != "First change\n\nMessage body" {
		t.Fatalf("commit message = %q", got)
	}
	files, err := WorkingChanges(ctx, dir)
	if err != nil || len(files) != 1 || files[0].Status != " M" {
		t.Fatalf("unstaged changes after commit: %+v, %v", files, err)
	}
	if err := CommitIndex(ctx, dir, "Should not commit"); err == nil {
		t.Fatal("committed unstaged changes")
	}
}

func TestCommitReportsHookFailureAndKeepsIndex(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "staged")
	if err := SetStaged(ctx, dir, "file.txt", true); err != nil {
		t.Fatal(err)
	}
	hooks := t.TempDir()
	command(t, dir, "config", "core.hooksPath", filepath.ToSlash(hooks))
	hook := "#!/bin/sh\nprintf 'fixture refused commit\n' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(hook), 0755); err != nil {
		t.Fatal(err)
	}
	if err := CommitIndex(ctx, dir, "Keep index"); err == nil || !strings.Contains(err.Error(), "fixture refused commit") {
		t.Fatalf("hook error = %v", err)
	}
	files, err := WorkingChanges(ctx, dir)
	if err != nil || len(files) != 1 || files[0].Status != "A " {
		t.Fatalf("index after hook failure: %+v, %v", files, err)
	}
}
