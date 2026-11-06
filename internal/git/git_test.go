package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func command(t testing.TB, dir string, args ...string) string {
	t.Helper()
	out, err := run(context.Background(), dir, nil, 1<<20, args...)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestDiffPreviewLimitsAndTextFidelity(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	content := "first\r\nlast"
	lfs := "version https://git-lfs.github.com/spec/v1\noid sha256:" + strings.Repeat("a", 64) + "\nsize 123\n"
	write(t, dir, "crlf.txt", content)
	write(t, dir, "pointer.txt", lfs)
	write(t, dir, "large.txt", strings.Repeat("x", 2*previewBytes+1024))
	commit(t, dir, "preview cases")
	hash := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	files, err := Files(ctx, dir, hash, 0)
	if err != nil || len(files) != 3 {
		t.Fatalf("files: %+v, %v", files, err)
	}
	for _, test := range []struct{ path, kind, text string }{
		{"crlf.txt", "text", content},
		{"large.txt", "large", ""},
		{"pointer.txt", "lfs", lfs},
	} {
		t.Run(test.path, func(t *testing.T) {
			for _, file := range files {
				if file.Path == test.path {
					result, err := Diff(ctx, dir, hash, 0, file)
					if err != nil || result.Kind != test.kind || result.After != test.text {
						t.Fatalf("kind=%q, size=%d, err=%v", result.Kind, len(result.After), err)
					}
					return
				}
			}
			t.Fatal("file missing")
		})
	}
}

func repository(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	command(t, dir, "init", "-b", "main")
	command(t, dir, "config", "user.name", "Test Author")
	command(t, dir, "config", "user.email", "test@example.invalid")
	command(t, dir, "config", "commit.gpgsign", "false")
	command(t, dir, "config", "core.autocrlf", "false")
	return dir
}

func write(t testing.TB, dir, path, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func commit(t testing.TB, dir, title string) {
	t.Helper()
	command(t, dir, "add", "-A")
	command(t, dir, "commit", "-m", title)
}

func TestOpenAndEmptyHistory(t *testing.T) {
	ctx := context.Background()
	if _, err := Open(ctx, t.TempDir()); err == nil {
		t.Fatal("accepted a non-repository")
	}
	dir := repository(t)
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	repo, err := Open(ctx, sub)
	if err != nil || repo.Head != "main" {
		t.Fatalf("open subdirectory: %+v, %v", repo, err)
	}
	wantRoot, wantErr := os.Stat(dir)
	gotRoot, gotErr := os.Stat(repo.Path)
	if wantErr != nil || gotErr != nil || !os.SameFile(wantRoot, gotRoot) {
		t.Fatalf("expected repository root %q, got %q", dir, repo.Path)
	}
	page, err := History(ctx, repo.Path, Cursor{})
	if err != nil || len(page.Commits) != 0 || page.More {
		t.Fatalf("unborn history: %+v, %v", page, err)
	}
}

func TestHistoryMergeAndRenameDiff(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	old, renamed := "旧 [1].txt", "新 [1].txt"
	content := "first\nsecond\nthird\nfourth\nfifth"
	write(t, dir, old, content)
	commit(t, dir, "root")
	command(t, dir, "tag", "v1")
	command(t, dir, "switch", "-c", "feature")
	write(t, dir, "data.bin", "abc\x00def")
	commit(t, dir, "feature")
	command(t, dir, "switch", "main")
	command(t, dir, "mv", "--", old, renamed)
	commit(t, dir, "rename")
	command(t, dir, "merge", "--no-ff", "feature", "-m", "merge")

	page, err := History(ctx, dir, Cursor{})
	if err != nil || len(page.Commits) != 4 || len(page.Commits[0].Parents) != 2 ||
		page.Commits[3].Subject != "root" || len(page.Commits[3].Refs) != 1 {
		t.Fatalf("unexpected history: %+v, %v", page, err)
	}
	merge, rename, root := page.Commits[0], page.Commits[0].Parents[0], page.Commits[3]
	for _, test := range []struct {
		hash                              string
		parent                            int
		status, path, before, after, kind string
	}{
		{root.Hash, 0, "A", old, "", content, "text"},
		{rename, 0, "R", renamed, content, content, "text"},
		{merge.Hash, 0, "A", "data.bin", "", "", "binary"},
		{merge.Hash, 1, "R", renamed, content, content, "text"},
	} {
		files, err := Files(ctx, dir, test.hash, test.parent)
		if err != nil || len(files) != 1 || files[0].Status != test.status || files[0].Path != test.path {
			t.Fatalf("unexpected files for %s: %+v, %v", test.path, files, err)
		}
		diff, err := Diff(ctx, dir, test.hash, test.parent, files[0])
		if err != nil || diff.Kind != test.kind || diff.Before != test.before || diff.After != test.after {
			t.Fatalf("unexpected diff for %s: %+v, %v", test.path, diff, err)
		}
	}
}

func TestDetachedWorktreeAndShallowHistory(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "worktree.txt", "initial")
	commit(t, dir, "initial")
	write(t, dir, "worktree.txt", "updated")
	commit(t, dir, "update")

	worktree := filepath.Join(t.TempDir(), "detached")
	command(t, dir, "worktree", "add", "--detach", worktree, "HEAD")
	repo, err := Open(ctx, worktree)
	if err != nil || repo.Name != "detached" || repo.Head != "HEAD" {
		t.Fatalf("detached worktree: %+v, %v", repo, err)
	}

	shallow := filepath.Join(t.TempDir(), "shallow")
	command(t, dir, "clone", "--depth=1", "--no-local", dir, shallow)
	page, err := History(ctx, shallow, Cursor{})
	if err != nil || len(page.Commits) != 1 || len(page.Commits[0].Parents) != 0 {
		t.Fatalf("shallow boundary: %+v, %v", page, err)
	}
	files, err := Files(ctx, shallow, page.Commits[0].Hash, 0)
	if err != nil || len(files) != 1 || files[0].Status != "A" {
		t.Fatalf("shallow boundary files: %+v, %v", files, err)
	}
}

func TestGraphCarriesMergeLanes(t *testing.T) {
	commits := []Commit{
		{Hash: "M", Parents: []string{"B", "C"}},
		{Hash: "B", Parents: []string{"A"}},
		{Hash: "C", Parents: []string{"A"}},
		{Hash: "A"},
	}
	lanes := layout(commits[:1], nil)
	if len(lanes) != 2 || len(commits[0].Graph.Out) != 2 {
		t.Fatalf("merge must emit two continuing tracks: %+v", commits[0].Graph)
	}
	lanes = layout(commits[1:], lanes)
	if len(lanes) != 0 || commits[2].Graph.Node != 1 || commits[3].Graph.Node != 0 {
		t.Fatalf("tracks must converge at root: %+v, %v", commits, lanes)
	}
}
