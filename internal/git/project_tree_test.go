package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectTreeManagesEmptyFoldersAndRejectsStaleTargets(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	if err := ChangeProjectEntry(ctx, dir, "folder", "", "", ".", "empty"); err != nil {
		t.Fatal(err)
	}
	if err := ChangeProjectEntry(ctx, dir, "file", "", "", "empty", "sample.txt"); err != nil {
		t.Fatal(err)
	}
	entries, err := ProjectEntries(dir, "empty")
	if err != nil || len(entries) != 1 || entries[0].Path != "empty/sample.txt" {
		t.Fatalf("entries: %+v, %v", entries, err)
	}
	file := entries[0]
	write(t, dir, file.Path, "changed on disk")
	if err := ChangeProjectEntry(ctx, dir, "delete", file.Path, file.Revision, "", ""); err == nil {
		t.Fatal("deleted a changed file using a stale confirmation")
	}
	entries, err = ProjectEntries(dir, "empty")
	if err != nil || len(entries) != 1 {
		t.Fatalf("reloaded entries: %+v, %v", entries, err)
	}
	file = entries[0]
	if err := ChangeProjectEntry(ctx, dir, "copy", file.Path, file.Revision, ".", "sample.txt"); err != nil {
		t.Fatal(err)
	}
	if err := ChangeProjectEntry(ctx, dir, "move", file.Path, file.Revision, ".", "sample.txt"); err == nil {
		t.Fatal("overwrote an existing file")
	}
	if err := ChangeProjectEntry(ctx, dir, "move", file.Path, file.Revision, ".", "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "renamed.txt"); got != "changed on disk" {
		t.Fatalf("moved content: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "empty", "sample.txt")); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	matches, err := SearchProject(ctx, dir, ".", "changed on disk")
	if err != nil || len(matches) != 2 || matches[0].Line != 1 {
		t.Fatalf("search matches: %+v, %v", matches, err)
	}
	for _, name := range []string{"..", "a/b", ".git", "a\\b", "file:stream", "CON"} {
		if err := ChangeProjectEntry(ctx, dir, "file", "", "", ".", name); err == nil {
			t.Fatalf("created an unsafe name %q", name)
		}
	}
}
