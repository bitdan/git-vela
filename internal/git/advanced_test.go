package git

import (
	"context"
	"strings"
	"testing"
)

func TestFileHistoryAndBlame(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "old [name].txt", "first\nsecond\n")
	commit(t, dir, "original file")
	command(t, dir, "mv", "--", "old [name].txt", "new [name].txt")
	commit(t, dir, "rename")
	write(t, dir, "new [name].txt", "first\nlocal change\n")

	history, err := FileHistory(ctx, dir, "new [name].txt")
	if err != nil || len(history.Commits) != 2 || history.Commits[1].Subject != "original file" {
		t.Fatalf("file history: %+v, %v", history, err)
	}
	blame, err := FileBlame(ctx, dir, "new [name].txt")
	if err != nil || len(blame) != 2 || blame[0].Number != 1 || blame[0].Author != "Test Author" ||
		!blame[1].Uncommitted || blame[1].Text != "local change" {
		t.Fatalf("file blame: %+v, %v", blame, err)
	}
	if _, err := FileBlame(ctx, dir, ".git/config"); err == nil {
		t.Fatal("blame accepted Git metadata")
	}
}

func TestCompareCommitTrees(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "old.txt", "same content")
	commit(t, dir, "base")
	base := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	command(t, dir, "mv", "old.txt", "renamed.txt")
	write(t, dir, "other.txt", "new content")
	commit(t, dir, "tip")
	tip := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	files, err := CompareFiles(ctx, dir, base, tip)
	if err != nil || len(files) != 2 {
		t.Fatalf("compare files: %+v, %v", files, err)
	}
	for _, file := range files {
		diff, err := CompareDiff(ctx, dir, base, tip, file.Index)
		if err != nil || diff.Kind != "text" {
			t.Fatalf("compare diff: %+v, %v", diff, err)
		}
		if file.Path == "renamed.txt" && (file.OldPath != "old.txt" || diff.Before != diff.After) {
			t.Fatalf("rename not preserved: %+v %+v", file, diff)
		}
	}
}

func TestCherryPickConflictCanResolveContinueAndRevert(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "base\n")
	commit(t, dir, "base")
	command(t, dir, "switch", "-c", "side")
	write(t, dir, "file.txt", "side\n")
	commit(t, dir, "side")
	side := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	command(t, dir, "switch", "main")
	write(t, dir, "file.txt", "main\n")
	commit(t, dir, "main")
	head, err := ReadHead(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHistoryOperation(ctx, dir, "cherry-pick", side, head, ""); err == nil {
		t.Fatal("expected cherry-pick conflict")
	}
	state, err := ReadOperationState(ctx, dir)
	if err != nil || state.Kind != "cherry-pick" || len(state.Conflicts) != 1 || state.Conflicts[0] != "file.txt" {
		t.Fatalf("operation state: %+v, %v", state, err)
	}
	if err := FinishHistoryOperation(ctx, dir, "cherry-pick", false); err == nil {
		t.Fatal("continued with unmerged files")
	}
	versions, err := ConflictVersions(ctx, dir, "file.txt")
	if err != nil || len(versions) != 3 || versions[0].Content != "base\n" ||
		versions[1].Content != "main\n" || versions[2].Content != "side\n" {
		t.Fatalf("conflict versions: %+v, %v", versions, err)
	}
	write(t, dir, "file.txt", "resolved\n")
	if err := ResolveConflict(ctx, dir, "file.txt"); err != nil {
		t.Fatal(err)
	}
	if err := FinishHistoryOperation(ctx, dir, "cherry-pick", false); err != nil {
		t.Fatal(err)
	}
	newHead, err := ReadHead(ctx, dir)
	if err != nil || head.Hash == newHead.Hash {
		t.Fatalf("cherry-pick HEAD: %+v, %v", newHead, err)
	}
	if err := ApplyHistoryOperation(ctx, dir, "revert", newHead.Hash, head, ""); err == nil {
		t.Fatal("accepted a stale confirmation")
	}
	if err := ApplyHistoryOperation(ctx, dir, "revert", newHead.Hash, newHead, ""); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "show", "HEAD:file.txt"); got != "main\n" {
		t.Fatalf("revert restored %q", got)
	}
}

func TestResetModesProtectIgnoredFiles(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "main.txt", "base")
	commit(t, dir, "base")
	base, _ := ReadHead(ctx, dir)
	write(t, dir, "local.txt", "tracked at tip")
	commit(t, dir, "tip")
	tip, _ := ReadHead(ctx, dir)
	if err := ApplyHistoryOperation(ctx, dir, "reset", base.Hash, tip, "soft"); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "diff", "--cached", "--name-only"); strings.TrimSpace(got) != "local.txt" {
		t.Fatalf("soft reset lost staged content: %q", got)
	}
	if err := ApplyHistoryOperation(ctx, dir, "reset", base.Hash, base, "mixed"); err != nil {
		t.Fatal(err)
	}
	write(t, dir, ".git/info/exclude", "local.txt\n")
	write(t, dir, "local.txt", "ignored data")
	if err := ApplyHistoryOperation(ctx, dir, "reset", tip.Hash, base, "hard"); err == nil {
		t.Fatal("hard reset overwrote ignored data")
	}
	file, err := WorkingPreview(dir, "local.txt")
	if err != nil || file.Content != "ignored data" {
		t.Fatalf("ignored file: %+v, %v", file, err)
	}
}

func TestStageAndUnstageIndependentHunks(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	original := "first\n" + strings.Repeat("context\n", 30) + "last\n"
	write(t, dir, "file [x].txt", original)
	commit(t, dir, "initial")
	changed := strings.Replace(strings.Replace(original, "first", "edited first", 1), "last", "edited last", 1)
	write(t, dir, "file [x].txt", changed)
	page, err := WorkingHunks(ctx, dir, "file [x].txt", false)
	if err != nil || len(page.Hunks) != 2 {
		t.Fatalf("working hunks: %+v, %v", page, err)
	}
	if err := SetHunkStaged(ctx, dir, "file [x].txt", false, page.Revision, 1); err != nil {
		t.Fatal(err)
	}
	staged := command(t, dir, "show", ":file [x].txt")
	if !strings.HasPrefix(staged, "first\n") || !strings.HasSuffix(staged, "edited last\n") {
		t.Fatalf("unexpected staged contents: %q", staged)
	}
	if err := SetHunkStaged(ctx, dir, "file [x].txt", false, page.Revision, 0); err == nil {
		t.Fatal("accepted stale hunk selection")
	}
	page, err = WorkingHunks(ctx, dir, "file [x].txt", true)
	if err != nil || len(page.Hunks) != 1 {
		t.Fatalf("staged hunks: %+v, %v", page, err)
	}
	if err := SetHunkStaged(ctx, dir, "file [x].txt", true, page.Revision, 0); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "show", ":file [x].txt"); got != original {
		t.Fatalf("unstage changed the index unexpectedly: %q", got)
	}
	preview, err := WorkingPreview(dir, "file [x].txt")
	if err != nil || preview.Content != changed {
		t.Fatalf("hunk stage touched working contents: %+v, %v", preview, err)
	}
}

func TestStageSelectedHunkLines(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "start\nold one\nold two\nend\n")
	commit(t, dir, "initial")
	write(t, dir, "file.txt", "start\nnew one\nnew two\nend\n")
	page, err := WorkingHunks(ctx, dir, "file.txt", false)
	if err != nil || len(page.Hunks) != 1 {
		t.Fatalf("patch: %+v, %v", page, err)
	}
	selected := patchLineIndexes(page.Hunks[0], "-old one", "+new one")
	if err := SetHunkLinesStaged(ctx, dir, "file.txt", false, page.Revision, 0, selected); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "show", ":file.txt"); got != "start\nold two\nnew one\nend\n" {
		t.Fatalf("unexpected partially staged contents: %q", got)
	}
	page, err = WorkingHunks(ctx, dir, "file.txt", true)
	if err != nil || len(page.Hunks) != 1 {
		t.Fatalf("patch: %+v, %v", page, err)
	}
	selected = patchLineIndexes(page.Hunks[0], "-old one", "+new one")
	if err := SetHunkLinesStaged(ctx, dir, "file.txt", true, page.Revision, 0, selected); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "show", ":file.txt"); got != "start\nold one\nold two\nend\n" {
		t.Fatalf("unexpected partially unstaged contents: %q", got)
	}
}

func patchLineIndexes(hunk string, wanted ...string) []int {
	var indexes []int
	for index, line := range strings.Split(hunk, "\n") {
		for _, match := range wanted {
			if line == match {
				indexes = append(indexes, index)
			}
		}
	}
	return indexes
}

func TestInteractiveRebaseReordersCommits(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "root.txt", "base")
	commit(t, dir, "base")
	base, _ := ReadHead(ctx, dir)
	write(t, dir, "one.txt", "one")
	commit(t, dir, "first")
	write(t, dir, "two.txt", "two")
	commit(t, dir, "second")
	head, _ := ReadHead(ctx, dir)
	plan, err := ReadRebasePlan(ctx, dir, base.Hash, head)
	if err != nil || len(plan) != 2 || plan[0].Subject != "first" || plan[1].Subject != "second" {
		t.Fatalf("rebase plan: %+v, %v", plan, err)
	}
	steps := []RebaseStep{{Hash: plan[1].Hash, Action: "pick"}, {Hash: plan[0].Hash, Action: "pick"}}
	if err := RunInteractiveRebase(ctx, dir, base.Hash, head, steps); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "log", "-2", "--format=%s"); got != "first\nsecond\n" {
		t.Fatalf("commits were not reordered: %q", got)
	}
}

func TestOpenInitializedSubmodule(t *testing.T) {
	ctx := context.Background()
	child := repository(t)
	write(t, child, "file.txt", "submodule")
	commit(t, child, "initial")
	dir := repository(t)
	command(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", "--", child, "modules/child")
	path, err := SubmodulePath(ctx, dir, "modules/child")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, path, "rev-parse", "HEAD")); got != strings.TrimSpace(command(t, child, "rev-parse", "HEAD")) {
		t.Fatalf("opened the wrong repository: %s", path)
	}
	if _, err := SubmodulePath(ctx, dir, "modules"); err == nil {
		t.Fatal("opened an ordinary directory as a submodule")
	}
	command(t, dir, "submodule", "deinit", "-f", "--", "modules/child")
	if _, err := SubmodulePath(ctx, dir, "modules/child"); err == nil {
		t.Fatal("opened an uninitialized submodule as its parent repository")
	}
}

func TestMergeAbortAndHardReset(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "base\n")
	commit(t, dir, "base")
	base, err := ReadHead(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	command(t, dir, "switch", "-c", "side")
	write(t, dir, "file.txt", "side\n")
	commit(t, dir, "side")
	side, err := ReadHead(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	command(t, dir, "switch", "main")
	write(t, dir, "file.txt", "main\n")
	commit(t, dir, "main")
	main, err := ReadHead(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHistoryOperation(ctx, dir, "merge", side.Hash, main, ""); err == nil {
		t.Fatal("expected a merge conflict")
	}
	state, err := ReadOperationState(ctx, dir)
	if err != nil || state.Kind != "merge" || len(state.Conflicts) != 1 {
		t.Fatalf("merge state: %+v, %v", state, err)
	}
	if err := FinishHistoryOperation(ctx, dir, "merge", true); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("aborting merge left changes: %q", got)
	}
	write(t, dir, "file.txt", "discard this\n")
	write(t, dir, "untracked.txt", "preserve this")
	if err := ApplyHistoryOperation(ctx, dir, "reset", base.Hash, main, "hard"); err != nil {
		t.Fatal(err)
	}
	preview, err := WorkingPreview(dir, "file.txt")
	if err != nil || preview.Content != "base\n" {
		t.Fatalf("hard reset result: %+v, %v", preview, err)
	}
	preview, err = WorkingPreview(dir, "untracked.txt")
	if err != nil || preview.Content != "preserve this" {
		t.Fatalf("hard reset touched unrelated untracked data: %+v, %v", preview, err)
	}
}
