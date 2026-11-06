package git

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSearchHistoryBeyondFirstPage(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	// fast-import builds a history larger than a UI page in a single Git process.
	var stream strings.Builder
	for i := 0; i < pageSize+2; i++ {
		message := "needle."
		if i == 0 {
			message = "root [literal]"
		}
		stream.WriteString("commit refs/heads/main\n")
		if i == 0 {
			stream.WriteString("author Root Author <root@example.com> 946684800 +0000\n")
		}
		stream.WriteString("committer Fixture <fixture@example.com> 946771200 +0000\ndata " + strconv.Itoa(len(message)) + "\n" + message + "\n\n")
	}
	if _, err := run(ctx, dir, []byte(stream.String()), 1<<20, "fast-import", "--quiet"); err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(command(t, dir, "rev-list", "--max-parents=0", "main"))
	command(t, dir, "branch", "root-only", root)

	query := HistoryFilter{Message: "[literal]", Author: "ROOT@EXAMPLE.COM", Branch: "refs/heads/main",
		From: "2000-01-02T00:00:00Z", Through: "2000-01-02T23:59:59Z"}
	result, err := SearchHistory(ctx, dir, query, Cursor{})
	if err != nil || len(result.Commits) != 1 || result.Commits[0].Hash != root || result.Commits[0].Time != 946771200 {
		t.Fatalf("search result: %+v, %v", result, err)
	}
	query.Through = "2000-01-01T23:59:59Z"
	if _, err := SearchHistory(ctx, dir, query, Cursor{}); err == nil {
		t.Fatal("accepted a reversed date range")
	}

	query = HistoryFilter{Message: "needle."}
	first, err := SearchHistory(ctx, dir, query, Cursor{})
	if err != nil || !first.More || len(first.Commits) != pageSize {
		t.Fatalf("first page: count=%d more=%t err=%v", len(first.Commits), first.More, err)
	}
	last, err := SearchHistory(ctx, dir, query, first.Next)
	if err != nil || last.More || len(last.Commits) != 1 || last.Commits[0].Hash == first.Commits[pageSize-1].Hash {
		t.Fatalf("last page: count=%d more=%t err=%v", len(last.Commits), last.More, err)
	}
	if _, err := SearchHistory(ctx, dir, HistoryFilter{Branch: "refs/heads/root-only"}, first.Next); err == nil {
		t.Fatal("accepted a cursor from a different search")
	}
	result, err = SearchHistory(ctx, dir, HistoryFilter{Branch: "refs/heads/root-only"}, Cursor{})
	if err != nil || len(result.Commits) != 1 || result.Commits[0].Hash != root {
		t.Fatalf("branch-filtered result: %+v, %v", result, err)
	}
	result, err = SearchHistory(ctx, dir, HistoryFilter{Hash: root[:10]}, Cursor{})
	if err != nil || len(result.Commits) != 1 || result.Commits[0].Hash != root {
		t.Fatalf("hash result: %+v, %v", result, err)
	}
	command(t, dir, "branch", "external-change", root)
	if _, err := SearchHistory(ctx, dir, query, first.Next); err == nil {
		t.Fatal("accepted a search cursor after refs changed")
	}
}

func TestReadCommitMessage(t *testing.T) {
	dir := repository(t)
	write(t, dir, "file.txt", "initial")
	command(t, dir, "add", "--", "file.txt")
	command(t, dir, "commit", "-m", "Subject", "-m", "Second paragraph.")
	hash := strings.TrimSpace(command(t, dir, "rev-parse", "HEAD"))
	message, err := ReadCommitMessage(context.Background(), dir, hash)
	if err != nil || strings.TrimSpace(message) != "Subject\n\nSecond paragraph." {
		t.Fatalf("message = %q, error = %v", message, err)
	}
}
