package git

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEditWorkingTextPreservesEncodingAndDoesNotStage(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "\ufefffirst\r\nsecond")
	commit(t, dir, "initial")
	file, err := OpenTextFile(ctx, dir, "file.txt")
	if err != nil || file.Content != "first\nsecond" || file.LineEnding != "CRLF" {
		t.Fatalf("open: %+v, %v", file, err)
	}
	revision, err := SaveTextFile(ctx, dir, "file.txt", file.Revision, "first\nupdated")
	if err != nil || revision == file.Revision {
		t.Fatalf("save: %q, %v", revision, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil || string(data) != "\ufefffirst\r\nupdated" {
		t.Fatalf("saved data = %q, %v", data, err)
	}
	if got := command(t, dir, "status", "--porcelain"); got != " M file.txt\n" {
		t.Fatalf("editing affected Git index: %q", got)
	}
	write(t, dir, "file.txt", "externally changed")
	if _, err := SaveTextFile(ctx, dir, "file.txt", revision, "stale edit"); err == nil {
		t.Fatal("overwrote an external change")
	}
	data, err = os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil || string(data) != "externally changed" {
		t.Fatalf("external change was lost: %q, %v", data, err)
	}
}

func TestEditableFilesExcludeIgnoredPathsAndRejectUnsafeText(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, ".gitignore", "ignored.txt\n")
	write(t, dir, "tracked.txt", "tracked")
	commit(t, dir, "initial")
	write(t, dir, "ignored.txt", "private")
	write(t, dir, "中文.txt", "untracked")
	write(t, dir, "binary", "\x00")
	write(t, dir, "mixed.txt", "a\r\nb\nc")
	files, err := EditableFilePaths(ctx, dir)
	if err != nil || !reflect.DeepEqual(files, []string{".gitignore", "binary", "mixed.txt", "tracked.txt", "中文.txt"}) {
		t.Fatalf("files = %#v, %v", files, err)
	}
	for _, path := range []string{".git/config", "../outside", "binary", "mixed.txt", "missing.txt"} {
		if _, err := OpenTextFile(ctx, dir, path); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
}
