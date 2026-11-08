package git

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestTagLifecycle(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "base")
	commit(t, dir, "base")
	head := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))

	if err := CreateTag(ctx, dir, "", head, ""); err == nil {
		t.Fatal("accepted an empty tag name")
	}
	if err := CreateTag(ctx, dir, "-evil", head, ""); err == nil {
		t.Fatal("accepted a flag-looking tag name")
	}
	if err := CreateTag(ctx, dir, "bad..name", head, ""); err == nil {
		t.Fatal("accepted an invalid refname")
	}
	if err := CreateTag(ctx, dir, "v1.0", "deadbeef", ""); err == nil {
		t.Fatal("accepted a non-commit target")
	}
	if err := CreateTag(ctx, dir, "v1.0", head, ""); err != nil {
		t.Fatal(err)
	}
	if err := CreateTag(ctx, dir, "v1.0", head, "again"); err == nil {
		t.Fatal("overwrote an existing tag")
	}
	if err := CreateTag(ctx, dir, "signed-ish", head, "annotated note"); err != nil {
		t.Fatal(err)
	}
	if kind := strings.TrimSpace(command(t, dir, "for-each-ref", "--format=%(objecttype)", "refs/tags/signed-ish")); kind != "tag" {
		t.Fatalf("annotated tag object type: %q", kind)
	}
	if kind := strings.TrimSpace(command(t, dir, "for-each-ref", "--format=%(objecttype)", "refs/tags/v1.0")); kind != "commit" {
		t.Fatalf("lightweight tag object type: %q", kind)
	}

	// Checkout a tag into a detached HEAD (nested tag names validate too).
	if err := CreateTag(ctx, dir, "release/v1", head, ""); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "file.txt", "second")
	commit(t, dir, "second")
	write(t, dir, "file.txt", "dirty")
	// file.txt differs between HEAD and the tag, so the dirty change conflicts.
	if err := CheckoutTag(ctx, dir, "v1.0"); err == nil {
		t.Fatal("checked out over a conflicting dirty worktree")
	}
	command(t, dir, "restore", "file.txt")
	if err := CheckoutTag(ctx, dir, "missing"); err == nil {
		t.Fatal("checked out an unknown tag")
	}
	if err := CheckoutTag(ctx, dir, "release/v1"); err != nil {
		t.Fatal(err)
	}
	if state := strings.TrimSpace(command(t, dir, "status", "--porcelain", "--branch")); !strings.Contains(state, "HEAD (no branch)") {
		t.Fatalf("expected detached HEAD after tag checkout: %q", state)
	}

	if err := DeleteTag(ctx, dir, "v1.0"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteTag(ctx, dir, "v1.0"); err == nil {
		t.Fatal("deleted an unknown tag")
	}
	if refs := command(t, dir, "for-each-ref", "refs/tags"); strings.Contains(refs, "v1.0") {
		t.Fatalf("tag survived deletion: %q", refs)
	}
}

func TestPushTagProtectsRemoteAndValidatesRemote(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	command(t, dir, "init", "--bare", "-b", "main", bare)
	command(t, dir, "remote", "add", "origin", bare)
	write(t, dir, "file.txt", "base")
	commit(t, dir, "base")
	command(t, dir, "tag", "v1.0")

	if err := PushTag(ctx, dir, "upstream", "v1.0"); err == nil {
		t.Fatal("accepted an unconfigured remote")
	}
	if err := PushTag(ctx, dir, "origin", "missing"); err == nil {
		t.Fatal("pushed an unknown tag")
	}
	if err := PushTag(ctx, dir, "origin", "v1.0"); err != nil {
		t.Fatal(err)
	}
	if got := command(t, bare, "for-each-ref", "refs/tags"); !strings.Contains(got, "v1.0") {
		t.Fatalf("tag missing on the remote: %q", got)
	}

	// Moving a published tag must not be possible through the safe path.
	command(t, dir, "tag", "-f", "-m", "moved", "v1.0")
	if err := PushTag(ctx, dir, "origin", "v1.0"); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("forced tag update rejected: %v", err)
	}
}

func TestPullModesMergeRebaseAutostash(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	command(t, dir, "init", "--bare", "-b", "main", bare)
	command(t, dir, "remote", "add", "origin", bare)
	write(t, dir, "file.txt", "base")
	commit(t, dir, "base")
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{}); err != nil {
		t.Fatal(err)
	}

	peer := repository(t)
	command(t, peer, "remote", "add", "origin", bare)
	command(t, peer, "pull", "origin", "main")
	write(t, peer, "peer.txt", "peer")
	commit(t, peer, "peer update")
	command(t, peer, "push", "origin", "main")

	// Rebase mode: local commit is replayed on top of the peer commit.
	write(t, dir, "local.txt", "local")
	commit(t, dir, "local update")
	if err := FetchRemote(ctx, dir, "origin"); err != nil {
		t.Fatal(err)
	}
	if err := PullUpstream(ctx, dir, "main", "origin", "main", PullOptions{Mode: PullRebase}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "log", "--format=%s")); !strings.HasPrefix(got, "local update\npeer update") {
		t.Fatalf("rebase pull ordering: %q", got)
	}

	// Merge mode: divergence produces a merge commit containing both lines.
	write(t, peer, "peer.txt", "more")
	commit(t, peer, "another peer update")
	command(t, peer, "push", "origin", "main")
	write(t, dir, "local.txt", "second local")
	commit(t, dir, "second local")
	if err := FetchRemote(ctx, dir, "origin"); err != nil {
		t.Fatal(err)
	}
	if err := PullUpstream(ctx, dir, "main", "origin", "main", PullOptions{Mode: PullMerge}); err != nil {
		t.Fatal(err)
	}
	if parents := strings.Fields(command(t, dir, "log", "-1", "--format=%P")); len(parents) != 2 {
		t.Fatalf("merge pull produced %d parents", len(parents))
	}

	// Autostash: dirty tracked file survives a merge pull.
	write(t, peer, "peer.txt", "third")
	commit(t, peer, "third peer update")
	command(t, peer, "push", "origin", "main")
	if err := FetchRemote(ctx, dir, "origin"); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "local.txt", "unsaved edits")
	if err := PullUpstream(ctx, dir, "main", "origin", "main", PullOptions{Mode: PullMerge, Autostash: true}); err != nil {
		t.Fatal(err)
	}
	if content, err := WorkingPreview(dir, "local.txt"); err != nil || content.Content != "unsaved edits" {
		t.Fatalf("autostash lost the working edit: %+v, %v", content, err)
	}

	// Unknown mode rejected; autostash=false still guards a dirty worktree.
	write(t, dir, "local.txt", "still dirty")
	if err := PullUpstream(ctx, dir, "main", "origin", "main", PullOptions{Mode: "cherry"}); err == nil {
		t.Fatal("accepted an unknown pull mode")
	}
	if err := PullUpstream(ctx, dir, "main", "origin", "main", PullOptions{Mode: PullMerge}); err == nil {
		t.Fatal("merged into a dirty worktree without autostash")
	}
}

func TestPushOptionsUpstreamLease(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	command(t, dir, "init", "--bare", "-b", "main", bare)
	command(t, dir, "remote", "add", "origin", bare)
	write(t, dir, "file.txt", "base")
	commit(t, dir, "base")
	write(t, dir, "file.txt", "second")
	commit(t, dir, "second")
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{}); err != nil {
		t.Fatal(err)
	}

	// Pushing to a differently named remote branch needs an explicit upstream.
	command(t, dir, "switch", "-c", "feature")
	if err := PushBranch(ctx, dir, "feature", "origin", "preview", PushOptions{}); err == nil {
		t.Fatal("pushed to a mismatched target without an upstream option")
	}
	if err := PushBranch(ctx, dir, "feature", "origin", "preview", PushOptions{SetUpstream: true}); err != nil {
		t.Fatal(err)
	}
	if ref := strings.TrimSpace(command(t, dir, "rev-parse", "--abbrev-ref", "feature@{upstream}")); ref != "origin/preview" {
		t.Fatalf("explicit upstream not stored: %q", ref)
	}

	// Force-with-lease requires a remote-tracking ref for the target.
	command(t, dir, "update-ref", "-d", "refs/remotes/origin/preview")
	if err := PushBranch(ctx, dir, "feature", "origin", "preview", PushOptions{SetUpstream: true, ForceWithLease: true}); err == nil ||
		!strings.Contains(err.Error(), "remote-tracking ref") {
		t.Fatalf("force-with-lease without tracking ref: %v", err)
	}

	// Rewrite local history, fetch, then force with the fresh lease: allowed.
	command(t, dir, "reset", "--hard", "HEAD~1")
	write(t, dir, "file.txt", "rewritten")
	commit(t, dir, "rewritten")
	if err := PushBranch(ctx, dir, "feature", "origin", "preview", PushOptions{SetUpstream: true}); err == nil ||
		!strings.Contains(err.Error(), "rejected") {
		t.Fatalf("plain push should stay rejected after a rewrite: %v", err)
	}
	command(t, dir, "switch", "main")
	command(t, dir, "fetch", "origin", "+refs/heads/preview:refs/remotes/origin/preview")
	command(t, dir, "switch", "feature")
	if err := PushBranch(ctx, dir, "feature", "origin", "preview", PushOptions{SetUpstream: true, ForceWithLease: true}); err != nil {
		t.Fatal(err)
	}

	// A remote that moved beyond the lease must be rejected ("stale info").
	peer := t.TempDir()
	command(t, peer, "clone", "--quiet", bare, ".")
	command(t, peer, "config", "user.email", "peer@example.com")
	command(t, peer, "config", "user.name", "Peer")
	command(t, peer, "switch", "preview")
	write(t, peer, "file.txt", "peer moved the branch")
	commit(t, peer, "peer moves preview")
	command(t, peer, "push", "origin", "preview")
	if err := PushBranch(ctx, dir, "feature", "origin", "preview", PushOptions{SetUpstream: true, ForceWithLease: true}); err == nil ||
		!strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale lease push: %v", err)
	}
}

func TestBranchSyncCounts(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	command(t, dir, "init", "--bare", "-b", "main", bare)
	command(t, dir, "remote", "add", "origin", bare)
	write(t, dir, "file.txt", "base")
	commit(t, dir, "base")
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{}); err != nil {
		t.Fatal(err)
	}
	command(t, dir, "switch", "-c", "orphan-meta")

	entries, err := BranchSyncCounts(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	byBranch := map[string]BranchSync{}
	for _, entry := range entries {
		byBranch[entry.Branch] = entry
	}
	main, ok := byBranch["main"]
	if !ok || main.Upstream != "origin/main" || main.Ahead != 0 || main.Behind != 0 || main.Gone {
		t.Fatalf("main sync entry: %+v", main)
	}
	if entry, ok := byBranch["orphan-meta"]; !ok || entry.Upstream != "" {
		t.Fatalf("upstream-less branch entry: %+v (ok=%v)", entry, ok)
	}

	write(t, dir, "local.txt", "ahead")
	commit(t, dir, "local update")
	command(t, dir, "switch", "main")
	// Keep the merge commit local so the branch is ahead of the remote.
	command(t, dir, "merge", "--no-edit", "orphan-meta")
	peer := repository(t)
	command(t, peer, "remote", "add", "origin", bare)
	command(t, peer, "pull", "origin", "main")
	write(t, peer, "peer.txt", "peer")
	commit(t, peer, "peer update")
	command(t, peer, "push", "origin", "main")
	if err := FetchRemote(ctx, dir, "origin"); err != nil {
		t.Fatal(err)
	}

	entries, err = BranchSyncCounts(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	main = BranchSync{}
	for _, entry := range entries {
		if entry.Branch == "main" {
			main = entry
		}
	}
	if main.Ahead != 1 || main.Behind != 1 {
		t.Fatalf("expected ahead 1 behind 1, got %+v", main)
	}
}
