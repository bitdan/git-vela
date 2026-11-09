package git

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestBisectFindsTheFirstBadCommit(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	// Five commits; "bug" is introduced by the third one.
	for _, step := range []struct{ name, content string }{
		{"one", "ok\n"},
		{"two", "ok\n"},
		{"three", "bug\n"},
		{"four", "bug\n"},
		{"five", "bug\n"},
	} {
		write(t, dir, "file.txt", step.content)
		// A distinct file per step keeps every commit non-empty even when the
		// shared file content repeats.
		write(t, dir, step.name+".txt", step.name+"\n")
		commit(t, dir, step.name)
	}
	bad := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	good := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD~4"))
	culprit := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD~2"))

	if state, err := ReadBisectState(ctx, dir); err != nil || state.Active {
		t.Fatalf("inactive bisect: %+v, %v", state, err)
	}
	if err := MarkBisect(ctx, dir, "good"); err == nil {
		t.Fatal("marked a verdict without an active run")
	}
	if err := StartBisect(ctx, dir, good, bad); err == nil {
		t.Fatal("accepted endpoints in the wrong order")
	}
	if err := StartBisect(ctx, dir, bad, good); err != nil {
		t.Fatal(err)
	}
	if err := StartBisect(ctx, dir, bad, good); err == nil {
		t.Fatal("started a second run over an active one")
	}
	for attempt := 0; attempt < 8; attempt++ {
		state, err := ReadBisectState(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Active {
			t.Fatal("bisect ended without a reset")
		}
		if state.Finished {
			if state.FirstBad != culprit {
				t.Fatalf("first bad commit: got %s, want %s", state.FirstBad, culprit)
			}
			break
		}
		// The candidate commit's tree tells us whether it is good or bad.
		content := strings.TrimSpace(command(t, dir, "show", state.Current+":file.txt"))
		verdict := "good"
		if content == "bug" {
			verdict = "bad"
		}
		if err := MarkBisect(ctx, dir, verdict); err != nil {
			t.Fatal(err)
		}
	}
	if err := ResetBisect(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err := ResetBisect(ctx, dir); err == nil {
		t.Fatal("reset without an active run")
	}
	if got := strings.TrimSpace(command(t, dir, "symbolic-ref", "--short", "HEAD")); got != "main" {
		t.Fatalf("reset did not restore the branch: %q", got)
	}
}

func TestBisectRequiresACleanWorktree(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "one\n")
	commit(t, dir, "one")
	write(t, dir, "file.txt", "two\n")
	commit(t, dir, "two")
	bad := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	good := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD~1"))
	write(t, dir, "file.txt", "dirty\n")
	if err := StartBisect(ctx, dir, bad, good); err == nil {
		t.Fatal("started a bisect with a dirty worktree")
	}
}

func TestFetchAllRemotes(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "base\n")
	commit(t, dir, "base")
	first := filepath.Join(t.TempDir(), "first.git")
	second := filepath.Join(t.TempDir(), "second.git")
	for _, bare := range []string{first, second} {
		command(t, dir, "init", "--bare", "-b", "main", bare)
	}
	command(t, dir, "remote", "add", "first", first)
	command(t, dir, "remote", "add", "second", second)
	if err := FetchAllRemotes(ctx, dir, FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := PushBranch(ctx, dir, "main", "first", "main", PushOptions{SetUpstream: true}); err != nil {
		t.Fatal(err)
	}
	if err := PushBranch(ctx, dir, "main", "second", "main", PushOptions{SetUpstream: true}); err != nil {
		t.Fatal(err)
	}
	if err := FetchAllRemotes(ctx, dir, FetchOptions{Prune: true, Tags: FetchTagsNone}); err != nil {
		t.Fatal(err)
	}
	for _, remote := range []string{"first", "second"} {
		if refs := command(t, dir, "for-each-ref", "--format=%(refname)", "refs/remotes/"+remote); !strings.Contains(refs, remote+"/main") {
			t.Fatalf("%s/main missing after fetch --all: %q", remote, refs)
		}
	}
	if err := FetchAllRemotes(ctx, dir, FetchOptions{Tags: "sideways"}); err == nil {
		t.Fatal("accepted an unknown tag mode")
	}
	empty := repository(t)
	if err := FetchAllRemotes(ctx, empty, FetchOptions{}); err == nil {
		t.Fatal("fetched a repository without remotes")
	}
}

func TestProtectedBranchBlocksForcePush(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	command(t, dir, "init", "--bare", "-b", "main", bare)
	command(t, dir, "remote", "add", "origin", bare)
	write(t, dir, "file.txt", "base\n")
	commit(t, dir, "base")
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{SetUpstream: true}); err != nil {
		t.Fatal(err)
	}
	// Rewrite the pushed commit so a lease-bound force push is required.
	command(t, dir, "commit", "--amend", "-m", "rewritten")
	if err := FetchRemote(ctx, dir, "origin", FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{ForceWithLease: true, Protected: []string{"main"}}); err == nil ||
		!strings.Contains(err.Error(), "protected") {
		t.Fatalf("protected branch force push was not blocked: %v", err)
	}
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{ForceWithLease: true, Protected: []string{"release/*", " dev "}}); err != nil {
		t.Fatalf("unrelated protection blocked the push: %v", err)
	}
	if pattern, ok := protectedBranch([]string{"release/*", "main"}, "release/1.0"); !ok || pattern != "release/*" {
		t.Fatalf("wildcard protection: %q, %v", pattern, ok)
	}
}

func TestConfigureExecutable(t *testing.T) {
	ctx := context.Background()
	defer func() { _, _ = ConfigureExecutable(ctx, "") }()
	if version, err := ConfigureExecutable(ctx, ""); err != nil || !strings.Contains(version, "git version") {
		t.Fatalf("default executable: %q, %v", version, err)
	}
	if _, err := ConfigureExecutable(ctx, filepath.Join(t.TempDir(), "missing-git")); err == nil {
		t.Fatal("accepted a missing Git executable")
	}
	// A failed switch keeps the previous value usable.
	if version, err := Version(ctx); err != nil || !strings.Contains(version, "git version") {
		t.Fatalf("version after a failed switch: %q, %v", version, err)
	}
}

func TestRunGCKeepsHistory(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "one\n")
	commit(t, dir, "one")
	write(t, dir, "file.txt", "two\n")
	commit(t, dir, "two")
	if err := RunGC(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "rev-list", "--count", "HEAD")); got != "2" {
		t.Fatalf("gc dropped commits: %q", got)
	}
	if count, err := UnreachableCommits(ctx, dir); err != nil || count != 0 {
		t.Fatalf("unreachable commits after gc: %d, %v", count, err)
	}
}

func TestStartBisectAcceptsRevisionExpressions(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	for _, name := range []string{"one", "two", "three"} {
		write(t, dir, name+".txt", name+"\n")
		commit(t, dir, name)
	}
	// The panel pre-fills HEAD as the bad end, so a symbolic revision must work.
	if err := StartBisect(ctx, dir, "HEAD", "HEAD~2"); err != nil {
		t.Fatalf("HEAD as the bad endpoint: %v", err)
	}
	if state, err := ReadBisectState(ctx, dir); err != nil || !state.Active {
		t.Fatalf("active bisect after start: %+v, %v", state, err)
	}
	if err := ResetBisect(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err := StartBisect(ctx, dir, "HEAD", "no-such-revision"); err == nil {
		t.Fatal("accepted an unknown revision")
	}
}
