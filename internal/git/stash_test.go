package git

import (
	"context"
	"strings"
	"testing"
)

func TestPartialStashOnlyTakesSelectedFiles(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "a.txt", "base a\n")
	write(t, dir, "b.txt", "base b\n")
	commit(t, dir, "base")
	write(t, dir, "a.txt", "changed a\n")
	write(t, dir, "b.txt", "changed b\n")

	if err := SaveStash(ctx, dir, "only a", false, []string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "a.txt"); got != "base a\n" {
		t.Fatalf("stashed file was not restored: %q", got)
	}
	if got := read(t, dir, "b.txt"); got != "changed b\n" {
		t.Fatalf("unselected file was stashed: %q", got)
	}
	entries, err := Stashes(ctx, dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("stashes: %+v, %v", entries, err)
	}
	if !strings.Contains(entries[0].Subject, "only a") {
		t.Fatalf("stash subject: %q", entries[0].Subject)
	}
	// Applying restores the selected file only. The remaining edit in b.txt is
	// discarded first because apply requires a clean working tree.
	command(t, dir, "checkout", "--", "b.txt")
	if err := UpdateStash(ctx, dir, "apply", entries[0].Selector, entries[0].Hash, true); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "a.txt"); got != "changed a\n" {
		t.Fatalf("stash apply: %q", got)
	}

	for _, paths := range [][]string{{"../outside.txt"}, {"/absolute.txt"}, {"-u"}, {"a/../../b"}} {
		if err := SaveStash(ctx, dir, "", false, paths); err == nil {
			t.Fatalf("accepted unsafe path %v", paths)
		}
	}
}

func TestRenameStashRewritesTheMessage(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "file.txt", "base\n")
	commit(t, dir, "base")
	write(t, dir, "file.txt", "change\n")
	if err := SaveStash(ctx, dir, "original message", false, nil); err != nil {
		t.Fatal(err)
	}
	entries, err := Stashes(ctx, dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("stashes: %+v, %v", entries, err)
	}
	entry := entries[0]
	if err := RenameStash(ctx, dir, entry.Selector, entry.Hash, "renamed"); err != nil {
		t.Fatal(err)
	}
	renamed, err := Stashes(ctx, dir)
	if err != nil || len(renamed) != 1 {
		t.Fatalf("stashes after rename: %+v, %v", renamed, err)
	}
	if !strings.HasSuffix(renamed[0].Subject, "renamed") || strings.Contains(renamed[0].Subject, "original") {
		t.Fatalf("renamed subject: %q", renamed[0].Subject)
	}
	// The entry keeps its content and base commit.
	if renamed[0].Parent != entry.Parent {
		t.Fatalf("rename changed the base commit: %q → %q", entry.Parent, renamed[0].Parent)
	}
	if err := UpdateStash(ctx, dir, "pop", renamed[0].Selector, renamed[0].Hash, true); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "file.txt"); got != "change\n" {
		t.Fatalf("renamed stash did not apply: %q", got)
	}

	if err := RenameStash(ctx, dir, "stash@{0}", entry.Hash, ""); err == nil {
		t.Fatal("accepted an empty rename")
	}
	if err := RenameStash(ctx, dir, "stash@{0}", entry.Hash, "two\nlines"); err == nil {
		t.Fatal("accepted a multi-line rename")
	}
}
