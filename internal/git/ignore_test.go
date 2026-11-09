package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAddToGitignore(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "build", "output")
	write(t, dir, ".gitignore", "# generated\n")

	changed, err := AddToGitignore(ctx, dir, "build")
	if err != nil || !changed {
		t.Fatalf("add: changed=%v, %v", changed, err)
	}
	if got := read(t, dir, ".gitignore"); got != "# generated\n/build\n" {
		t.Fatalf(".gitignore = %q", got)
	}

	// A second call with the same file is a no-op, and a pattern that only
	// differs in spacing is still treated as present.
	changed, err = AddToGitignore(ctx, dir, "build")
	if err != nil || changed {
		t.Fatalf("second add: changed=%v, %v", changed, err)
	}
	if got := read(t, dir, ".gitignore"); got != "# generated\n/build\n" {
		t.Fatalf(".gitignore after duplicate = %q", got)
	}

	// Nested paths anchor at the repository root, with no trailing entry for
	// the directory itself.
	changed, err = AddToGitignore(ctx, dir, filepath.Join("logs", "sub", "out.txt"))
	if err != nil || !changed {
		t.Fatalf("nested add: changed=%v, %v", changed, err)
	}
	if got := read(t, dir, ".gitignore"); got != "# generated\n/build\n/logs/sub/out.txt\n" {
		t.Fatalf(".gitignore nested = %q", got)
	}

	// Creating the file when it is absent.
	write(t, dir, "temp.txt", "x")
	if err := os.Remove(filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	changed, err = AddToGitignore(ctx, dir, "temp.txt")
	if err != nil || !changed {
		t.Fatalf("fresh add: changed=%v, %v", changed, err)
	}
	if got := read(t, dir, ".gitignore"); got != "/temp.txt\n" {
		t.Fatalf(".gitignore fresh = %q", got)
	}

	// The ignored file drops out of the working status.
	files, err := WorkingChanges(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Path == "temp.txt" {
			t.Fatalf("temp.txt still listed after ignore: %+v", files)
		}
	}

	for _, path := range []string{"", "../outside.txt", "/abs.txt", "a/../b.txt", ".git/config"} {
		if _, err := AddToGitignore(ctx, dir, path); err == nil {
			t.Fatalf("accepted unsafe path %q", path)
		}
	}
}

func read(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
