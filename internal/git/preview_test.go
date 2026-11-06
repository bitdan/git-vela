package git

import (
	"context"
	"strings"
	"testing"
)

// A tiny PNG header is enough for the magic-byte detector; the payload is
// deliberately not valid UTF-8 so it also exercises the binary path.
const testPNG = "\x89PNG\r\n\x1a\n\x00\x01\x02\x03\xff\xfe\xfd\x00"

func TestImageAndExpandedDiffPreview(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	large := strings.Repeat("line\n", 120000)
	write(t, dir, "logo.png", testPNG)
	write(t, dir, "big.txt", large)
	write(t, dir, "raw.bin", "abc\x00def")
	commit(t, dir, "mixed preview")
	hash := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	files, err := Files(ctx, dir, hash, 0)
	if err != nil || len(files) != 3 {
		t.Fatalf("files: %+v, %v", files, err)
	}
	byPath := map[string]Change{}
	for _, file := range files {
		byPath[file.Path] = file
	}

	image, err := Diff(ctx, dir, hash, 0, byPath["logo.png"])
	if err != nil || image.Kind != "image" || !strings.HasPrefix(image.After, "data:image/png;base64,") {
		t.Fatalf("image preview: kind=%q, after=%q, err=%v", image.Kind, image.After[:min(len(image.After), 40)], err)
	}

	largeResult, err := Diff(ctx, dir, hash, 0, byPath["big.txt"])
	if err != nil || largeResult.Kind != "large" {
		t.Fatalf("expected a size limit, got %q, %v", largeResult.Kind, err)
	}
	expanded, err := DiffExpanded(ctx, dir, hash, 0, byPath["big.txt"])
	if err != nil || expanded.Kind != "text" || expanded.After != large {
		t.Fatalf("expanded preview: kind=%q, size=%d, err=%v", expanded.Kind, len(expanded.After), err)
	}

	binary, err := Diff(ctx, dir, hash, 0, byPath["raw.bin"])
	if err != nil || binary.Kind != "binary" {
		t.Fatalf("non-image binary: kind=%q, err=%v", binary.Kind, err)
	}
}

func TestWorkingImageAndExpandedPreview(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "base.txt", "base\n")
	commit(t, dir, "initial")
	large := strings.Repeat("line\n", 120000)
	write(t, dir, "logo.png", testPNG)
	write(t, dir, "big.txt", large)

	image, err := WorkingPreview(dir, "logo.png")
	if err != nil || image.Kind != "image" || !strings.HasPrefix(image.Content, "data:image/png;base64,") {
		t.Fatalf("working image: kind=%q, err=%v", image.Kind, err)
	}
	limited, err := WorkingPreview(dir, "big.txt")
	if err != nil || limited.Kind != "large" {
		t.Fatalf("working limit: kind=%q, err=%v", limited.Kind, err)
	}
	expanded, err := WorkingPreviewExpanded(dir, "big.txt")
	if err != nil || expanded.Kind != "text" || expanded.Content != large {
		t.Fatalf("expanded working preview: kind=%q, size=%d, err=%v", expanded.Kind, len(expanded.Content), err)
	}
	// The editor must still refuse images even though the preview can show them.
	if _, err := OpenTextFile(ctx, dir, "logo.png"); err == nil {
		t.Fatal("editor accepted an image file")
	}
}

func TestOctopusMergeGraphAndParentFiles(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "root.txt", "root\n")
	commit(t, dir, "root")
	root := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	for _, name := range []string{"one", "two", "three"} {
		command(t, dir, "switch", "-c", name, root)
		write(t, dir, name+".txt", name+"\n")
		commit(t, dir, name)
	}
	command(t, dir, "switch", "main")
	command(t, dir, "merge", "--no-ff", "-m", "octopus", "one", "two", "three")
	tip := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))

	page, err := History(ctx, dir, Cursor{})
	if err != nil || len(page.Commits) == 0 {
		t.Fatalf("history: %+v, %v", page, err)
	}
	if len(page.Commits[0].Parents) != 4 || page.Commits[0].Hash != tip {
		t.Fatalf("expected an octopus merge at the tip, got %+v", page.Commits[0])
	}
	if page.Commits[0].Graph.Width < 1 {
		t.Fatal("merge commit has no graph lane")
	}
	for parent := 0; parent < 4; parent++ {
		files, err := Files(ctx, dir, tip, parent)
		if err != nil || len(files) == 0 {
			t.Fatalf("parent %d changes: %+v, %v", parent, files, err)
		}
	}
}

func TestIndexLockIsReported(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "base\n")
	commit(t, dir, "initial")
	write(t, dir, "file.txt", "changed\n")
	write(t, dir, ".git/index.lock", "held by another Git process")
	if err := SetStaged(ctx, dir, "file.txt", true); err == nil {
		t.Fatal("staging silently ignored an existing index.lock")
	}
}
