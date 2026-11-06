package git

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchLinkedWorktree(t *testing.T) {
	dir := repository(t)
	write(t, dir, "file.txt", "initial")
	commit(t, dir, "initial")
	linked := filepath.Join(t.TempDir(), "linked")
	command(t, dir, "worktree", "add", "-b", "linked", linked)

	events := make(chan struct{}, 8)
	stop, err := WatchRepository(context.Background(), linked, func() { events <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	wait := func(what string) {
		t.Helper()
		select {
		case <-events:
		case <-time.After(5 * time.Second):
			t.Fatalf("no notification for %s", what)
		}
	}
	write(t, linked, "file.txt", "worktree edit")
	wait("worktree root edit")
	command(t, dir, "branch", "outside")
	wait("external shared ref")
	command(t, linked, "add", "--", "file.txt")
	wait("linked index update")
}
