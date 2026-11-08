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
	revision, err := SaveTextFile(ctx, dir, "file.txt", file.Revision, "first\nupdated", "")
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
	if _, err := SaveTextFile(ctx, dir, "file.txt", revision, "stale edit", ""); err == nil {
		t.Fatal("overwrote an external change")
	}
	data, err = os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil || string(data) != "externally changed" {
		t.Fatalf("external change was lost: %q, %v", data, err)
	}
}

func TestSaveTextFileSwitchesLineEndings(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "first\nsecond\n")
	commit(t, dir, "initial")
	file, err := OpenTextFile(ctx, dir, "file.txt")
	if err != nil || file.LineEnding != "LF" {
		t.Fatalf("open: %+v, %v", file, err)
	}
	// Same text with a new ending: the stored file converts, the revision moves.
	revision, err := SaveTextFile(ctx, dir, "file.txt", file.Revision, file.Content, "CRLF")
	if err != nil || revision == file.Revision {
		t.Fatalf("convert: %q, %v", revision, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil || string(data) != "first\r\nsecond\r\n" {
		t.Fatalf("converted data = %q, %v", data, err)
	}
	converted, err := OpenTextFile(ctx, dir, "file.txt")
	if err != nil || converted.LineEnding != "CRLF" || converted.Content != file.Content {
		t.Fatalf("reopened: %+v, %v", converted, err)
	}
	// An empty ending keeps the stored style; identical content is a no-op.
	same, err := SaveTextFile(ctx, dir, "file.txt", revision, file.Content, "")
	if err != nil || same != revision {
		t.Fatalf("no-op save: %q, %v", same, err)
	}
	// Reverting back to LF works too.
	back, err := SaveTextFile(ctx, dir, "file.txt", revision, file.Content, "LF")
	if err != nil || back == revision {
		t.Fatalf("revert: %q, %v", back, err)
	}
	data, err = os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil || string(data) != "first\nsecond\n" {
		t.Fatalf("reverted data = %q, %v", data, err)
	}
	if _, err := SaveTextFile(ctx, dir, "file.txt", back, file.Content, "CR"); err == nil {
		t.Fatal("accepted an unsupported line ending")
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
