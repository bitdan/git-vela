package git

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func importHistory(t testing.TB, count int) string {
	t.Helper()
	dir := repository(t)
	var script strings.Builder
	for index := 0; index < count; index++ {
		content := fmt.Sprintf("line %06d\n", index)
		fmt.Fprintf(&script, "commit refs/heads/main\ncommitter Test <test@example.invalid> %d +0000\ndata 4\nlog\nM 100644 inline sample.txt\ndata %d\n%s\n", 1700000000+index, len(content), content)
	}
	if _, err := run(context.Background(), dir, []byte(script.String()), 1<<20, "fast-import", "--quiet"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestHistoryPaginationAndExternalUpdate(t *testing.T) {
	ctx := context.Background()
	dir := importHistory(t, 203)
	first, err := History(ctx, dir, Cursor{})
	if err != nil || len(first.Commits) != 200 || !first.More {
		t.Fatalf("first page: %d commits, more=%v, err=%v", len(first.Commits), first.More, err)
	}
	second, err := History(ctx, dir, first.Next)
	if err != nil || len(second.Commits) != 3 || second.More {
		t.Fatalf("second page: %d commits, more=%v, err=%v", len(second.Commits), second.More, err)
	}
	last, next := first.Commits[199], second.Commits[0]
	if last.Parents[0] != next.Hash || !next.Graph.Incoming || last.Graph.Color != next.Graph.Color {
		t.Fatal("commit identity or graph color changed across the page boundary")
	}
	command(t, dir, "branch", "external-change")
	snapshot, err := Snapshot(ctx, dir)
	if err != nil || snapshot == first.Next.Snapshot {
		t.Fatalf("external branch change was missed: %q, %v", snapshot, err)
	}
	if _, err := History(ctx, dir, first.Next); err == nil {
		t.Fatal("accepted a cursor from before the reference change")
	}
}

func BenchmarkReadHistoryAndDiff(b *testing.B) {
	dir := importHistory(b, 10000)
	ctx := context.Background()
	first, err := History(ctx, dir, Cursor{})
	if err != nil {
		b.Fatal(err)
	}
	hash := first.Commits[0].Hash
	b.Run("First200Of10000", func(b *testing.B) {
		for b.Loop() {
			if _, err := History(ctx, dir, Cursor{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("SmallTextDiff", func(b *testing.B) {
		for b.Loop() {
			if _, err := Diff(ctx, dir, hash, 0, Change{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
