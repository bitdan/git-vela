package git

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAmendChecksExpectedHead(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	if _, err := PrepareAmend(ctx, dir); err == nil {
		t.Fatal("accepted amend in an unborn repository")
	}
	write(t, dir, "file.txt", "original")
	commit(t, dir, "original")
	first, err := PrepareAmend(ctx, dir)
	if err != nil || strings.TrimSpace(first.Message) != "original" || !objectID(first.Hash) {
		t.Fatalf("amend target: %+v, %v", first, err)
	}
	write(t, dir, "file.txt", "staged")
	command(t, dir, "add", "--", "file.txt")
	write(t, dir, "file.txt", "unstaged")
	if err := AmendIndex(ctx, dir, first.Hash, first.Ref, "updated", AmendOptions{}); err != nil {
		t.Fatal(err)
	}
	second, err := PrepareAmend(ctx, dir)
	if err != nil || second.Hash == first.Hash || strings.TrimSpace(second.Message) != "updated" {
		t.Fatalf("amended target: %+v, %v", second, err)
	}
	if command(t, dir, "show", "HEAD:file.txt") != "staged" {
		t.Fatal("amend included unstaged content")
	}
	if err := AmendIndex(ctx, dir, first.Hash, first.Ref, "stale", AmendOptions{}); err == nil {
		t.Fatal("amended a different HEAD")
	}
	command(t, dir, "switch", "-c", "same-tip")
	if err := AmendIndex(ctx, dir, second.Hash, second.Ref, "wrong branch", AmendOptions{}); err == nil {
		t.Fatal("amended a different branch at the same commit")
	}
	command(t, dir, "switch", "main")
	if err := AmendIndex(ctx, dir, second.Hash, second.Ref, "message only", AmendOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "rev-list", "--count", "HEAD")); got != "1" {
		t.Fatalf("amend created extra commits: %q", got)
	}
}

func TestStashRestoresIndexAndProtectsStaleEntry(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "base\n")
	commit(t, dir, "initial")
	write(t, dir, "file.txt", "staged\n")
	command(t, dir, "add", "--", "file.txt")
	write(t, dir, "file.txt", "working\n")
	write(t, dir, "新 untracked.txt", "new")
	if err := SaveStash(ctx, dir, "task work", true, nil); err != nil {
		t.Fatal(err)
	}
	command(t, dir, "config", "log.date", "iso")
	entries, err := Stashes(ctx, dir)
	if err != nil || len(entries) != 1 || entries[0].Selector != "stash@{0}" || !strings.Contains(entries[0].Subject, "task work") {
		t.Fatalf("stash list: %+v, %v", entries, err)
	}
	first := entries[0]
	if status := command(t, dir, "status", "--porcelain"); status != "" {
		t.Fatalf("stash left changes: %q", status)
	}
	write(t, dir, "file.txt", "newer\n")
	if err := SaveStash(ctx, dir, "newer", false, nil); err != nil {
		t.Fatal(err)
	}
	if err := UpdateStash(ctx, dir, "drop", first.Selector, first.Hash, false); err == nil {
		t.Fatal("dropped a stale stash selector")
	}
	if err := UpdateStash(ctx, dir, "apply", first.Selector, first.Hash, true); err != nil {
		t.Fatal(err)
	}
	if command(t, dir, "show", ":file.txt") != "staged\n" {
		t.Fatal("stash apply did not restore the index")
	}
	preview, err := WorkingPreview(dir, "file.txt")
	if err != nil || preview.Content != "working\n" {
		t.Fatalf("worktree content: %+v, %v", preview, err)
	}
	preview, err = WorkingPreview(dir, "新 untracked.txt")
	if err != nil || preview.Content != "new" {
		t.Fatalf("untracked content: %+v, %v", preview, err)
	}
	entries, err = Stashes(ctx, dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("apply changed stash list: %+v, %v", entries, err)
	}
	if err := UpdateStash(ctx, dir, "drop", entries[0].Selector, entries[0].Hash, false); err != nil {
		t.Fatal(err)
	}
	entries, err = Stashes(ctx, dir)
	if err != nil || len(entries) != 1 || entries[0].Hash != first.Hash {
		t.Fatalf("drop left unexpected entries: %+v, %v", entries, err)
	}
}

func TestSyncFastForwardAndRejectDivergence(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	command(t, dir, "init", "--bare", "-b", "main", bare)
	command(t, dir, "remote", "add", "origin", bare)
	write(t, dir, "local.txt", "base")
	commit(t, dir, "base")
	if err := FetchRemote(ctx, dir, "--all", FetchOptions{}); err == nil {
		t.Fatal("accepted an unconfigured remote")
	}
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{}); err != nil {
		t.Fatal(err)
	}
	state, err := ReadSyncState(ctx, dir)
	if err != nil || state.Branch != "main" || state.UpstreamRemote != "origin" || state.UpstreamBranch != "main" {
		t.Fatalf("sync state: %+v, %v", state, err)
	}

	peer := repository(t)
	command(t, peer, "remote", "add", "origin", bare)
	command(t, peer, "pull", "origin", "main")
	write(t, peer, "peer.txt", "new")
	commit(t, peer, "peer update")
	command(t, peer, "push", "origin", "main")
	if err := FetchRemote(ctx, dir, "origin", FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "log", "-1", "--format=%s")); got != "base" {
		t.Fatalf("fetch changed HEAD: %q", got)
	}
	write(t, dir, "local.txt", "unsaved")
	if err := PullUpstream(ctx, dir, "main", "origin", "main", PullOptions{}); err == nil {
		t.Fatal("pulled into a dirty worktree")
	}
	command(t, dir, "restore", "local.txt")
	write(t, dir, ".git/info/exclude", "peer.txt\n")
	write(t, dir, "peer.txt", "ignored local data")
	if err := PullUpstream(ctx, dir, "main", "origin", "main", PullOptions{}); err == nil {
		t.Fatal("pull overwrote an ignored file")
	}
	preview, err := WorkingPreview(dir, "peer.txt")
	if err != nil || preview.Content != "ignored local data" {
		t.Fatalf("ignored data changed: %+v, %v", preview, err)
	}
	if err := os.Remove(filepath.Join(dir, "peer.txt")); err != nil {
		t.Fatal(err)
	}
	if err := PullUpstream(ctx, dir, "main", "origin", "main", PullOptions{}); err != nil {
		t.Fatal(err)
	}

	write(t, dir, "local.txt", "local update")
	commit(t, dir, "local update")
	write(t, peer, "peer.txt", "another update")
	commit(t, peer, "another peer update")
	command(t, peer, "push", "origin", "main")
	if err := PullUpstream(ctx, dir, "main", "origin", "main", PullOptions{}); err == nil || !strings.Contains(err.Error(), "fast-forward") {
		t.Fatalf("diverged pull error: %v", err)
	}
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{}); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("non-fast-forward push error: %v", err)
	}
	if got := strings.TrimSpace(command(t, dir, "log", "-1", "--format=%s")); got != "local update" {
		t.Fatalf("diverged pull changed HEAD: %q", got)
	}
}

func TestStashPopKeepsConflictingEntry(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "base\n")
	commit(t, dir, "base")
	write(t, dir, "file.txt", "stashed\n")
	if err := SaveStash(ctx, dir, "saved edits", false, nil); err != nil {
		t.Fatal(err)
	}
	command(t, dir, "config", "log.date", "iso")
	entries, err := Stashes(ctx, dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("stash list: %+v, %v", entries, err)
	}
	entry := entries[0]
	write(t, dir, "file.txt", "conflicting commit\n")
	commit(t, dir, "conflicting commit")
	if err := UpdateStash(ctx, dir, "pop", entry.Selector, entry.Hash, false); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected a visible stash conflict, got %v", err)
	}
	entries, err = Stashes(ctx, dir)
	if err != nil || len(entries) != 1 || entries[0].Hash != entry.Hash {
		t.Fatalf("stash lost after conflict: %+v, %v", entries, err)
	}
}

func TestRemoteFailureRedactsCredentials(t *testing.T) {
	err := remoteFailure(errors.New("fatal: Authentication failed for 'https://user:secret@example.invalid/repo?token=private'"))
	if message := err.Error(); !strings.Contains(message, "credential helper") ||
		strings.Contains(message, "secret") || strings.Contains(message, "private") {
		t.Fatalf("unexpected authentication message: %q", message)
	}
}

func TestFetchUsesConfiguredCredentialHelper(t *testing.T) {
	ctx := context.Background()
	source := repository(t)
	write(t, source, "file.txt", "remote data")
	commit(t, source, "remote commit")
	bare := t.TempDir()
	command(t, source, "clone", "--bare", ".", bare)
	command(t, bare, "update-server-info")
	files := http.FileServer(http.Dir(bare))
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "gitvela-test" || password != "local-secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="GitVela fixture"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		files.ServeHTTP(w, r)
	}))
	defer remote.Close()

	client := repository(t)
	t.Setenv("GIT_ASKPASS", "")
	t.Setenv("SSH_ASKPASS", "")
	command(t, client, "config", "core.askPass", "")
	command(t, client, "config", "credential.helper", "")
	command(t, client, "remote", "add", "origin", remote.URL)
	if err := FetchRemote(ctx, client, "origin", FetchOptions{}); err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected actionable authentication failure, got %v", err)
	}
	command(t, client, "config", "--add", "credential.helper", "!f() { printf '%s\n' username=gitvela-test password=local-secret; }; f")
	if err := FetchRemote(ctx, client, "origin", FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, client, "log", "-1", "--format=%s", "origin/main")); got != "remote commit" {
		t.Fatalf("unexpected authenticated fetch result: %q", got)
	}
}

func TestCheckRepositoryPath(t *testing.T) {
	dir := repository(t)
	if status := CheckRepositoryPath(dir); !status.Available || status.Reason != "" {
		t.Fatalf("existing repository reported unavailable: %+v", status)
	}
	missing := filepath.Join(t.TempDir(), "deleted")
	status := CheckRepositoryPath(missing)
	if status.Available || !strings.Contains(status.Reason, "no longer available") {
		t.Fatalf("missing repository status: %+v", status)
	}
	empty := t.TempDir()
	status = CheckRepositoryPath(empty)
	if status.Available || !strings.Contains(status.Reason, "no longer contains a Git repository") {
		t.Fatalf("folder without Git metadata status: %+v", status)
	}
}

func TestAmendAuthorOptionsAndSignOff(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	command(t, dir, "config", "user.name", "Original Author")
	command(t, dir, "config", "user.email", "original@example.invalid")
	write(t, dir, "file.txt", "one\n")
	command(t, dir, "add", "-A")
	if _, err := runWithEnvironment(ctx, 2*time.Minute, dir, nil, 1<<20,
		[]string{"GIT_AUTHOR_DATE=2020-01-02T03:04:05+00:00", "GIT_COMMITTER_DATE=2020-01-02T03:04:05+00:00"},
		"commit", "-m", "first"); err != nil {
		t.Fatal(err)
	}
	wantDate := strings.TrimSpace(command(t, dir, "log", "-1", "--format=%at"))

	// A different identity amends the commit: Git keeps the author unless the
	// form asks to reset it, and the form must show the author it preserves.
	command(t, dir, "config", "user.name", "Test Author")
	command(t, dir, "config", "user.email", "test@example.invalid")
	target, err := PrepareAmend(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if target.Author != "Original Author" || target.AuthorEmail != "original@example.invalid" ||
		target.AuthorDate == 0 || strconv.FormatInt(target.AuthorDate, 10) != wantDate {
		t.Fatalf("amend target author: %+v (want date %s)", target, wantDate)
	}
	if err := AmendIndex(ctx, dir, target.Hash, target.Ref, "kept author", AmendOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "log", "-1", "--format=%an <%ae> %at")); !strings.HasPrefix(got, "Original Author <original@example.invalid> "+wantDate) {
		t.Fatalf("amend changed the author: %q", got)
	}

	// Reset author adopts the current identity.
	kept, err := PrepareAmend(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := AmendIndex(ctx, dir, kept.Hash, kept.Ref, "reset author", AmendOptions{ResetAuthor: true}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "log", "-1", "--format=%an <%ae>")); got != "Test Author <test@example.invalid>" {
		t.Fatalf("reset author kept the old identity: %q", got)
	}

	// Sign-off appends a trailer for the current committer identity.
	reset, err := PrepareAmend(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := AmendIndex(ctx, dir, reset.Hash, reset.Ref, "signed", AmendOptions{SignOff: true}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(command(t, dir, "log", "-1", "--format=%B")); !strings.HasSuffix(got, "Signed-off-by: Test Author <test@example.invalid>") {
		t.Fatalf("sign-off missing: %q", got)
	}
}

func TestCommitTemplateResolution(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "one\n")
	commit(t, dir, "first")

	if template, err := CommitTemplate(ctx, dir); err != nil || template != "" {
		t.Fatalf("unset template: %q, %v", template, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "docs/commit-template.txt", "Subject line\n\nBody\n")
	command(t, dir, "config", "commit.template", "docs/commit-template.txt")
	template, err := CommitTemplate(ctx, dir)
	if err != nil || template != "Subject line\n\nBody\n" {
		t.Fatalf("root-relative template: %q, %v", template, err)
	}
	absolute := filepath.Join(t.TempDir(), "absolute-template.txt")
	if err := os.WriteFile(absolute, []byte("Absolute\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command(t, dir, "config", "commit.template", absolute)
	if template, err := CommitTemplate(ctx, dir); err != nil || template != "Absolute\n" {
		t.Fatalf("absolute template: %q, %v", template, err)
	}
	command(t, dir, "config", "commit.template", "docs/missing.txt")
	if template, err := CommitTemplate(ctx, dir); err != nil || template != "" {
		t.Fatalf("missing template: %q, %v", template, err)
	}
}
