package git

import "testing"

func TestOutgoingCommitsListsWhatPushPublishes(t *testing.T) {
	ctx, dir, _, _ := syncFixture(t)
	if out, err := OutgoingCommits(ctx, dir, "main", "origin", "main"); err != nil || out.NewBranch || len(out.Commits) != 0 {
		t.Fatalf("up-to-date branch: %+v, %v", out, err)
	}

	write(t, dir, "one.txt", "1\n")
	commit(t, dir, "one")
	write(t, dir, "two.txt", "2\n")
	commit(t, dir, "two")
	out, err := OutgoingCommits(ctx, dir, "main", "origin", "main")
	if err != nil || out.NewBranch || len(out.Commits) != 2 || out.Commits[0].Subject != "two" || out.Tip != rev(t, dir, "main") {
		t.Fatalf("ahead branch: %+v, %v", out, err)
	}

	// A branch the remote does not have lists only commits on no remote branch.
	command(t, dir, "checkout", "-q", "-b", "topic")
	write(t, dir, "topic.txt", "t\n")
	commit(t, dir, "topic")
	out, err = OutgoingCommits(ctx, dir, "topic", "origin", "topic")
	if err != nil || !out.NewBranch || len(out.Commits) != 3 {
		t.Fatalf("new branch: %+v, %v", out, err)
	}

	for _, bad := range [][3]string{{"-x", "origin", "main"}, {"main", "nowhere", "main"}, {"main", "origin", "-f"}, {"missing", "origin", "missing"}} {
		if _, err := OutgoingCommits(ctx, dir, bad[0], bad[1], bad[2]); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}

func TestPushRefusesBranchThatMovedAfterReview(t *testing.T) {
	ctx, dir, _, bare := syncFixture(t)
	write(t, dir, "one.txt", "1\n")
	commit(t, dir, "one")
	reviewed, err := OutgoingCommits(ctx, dir, "main", "origin", "main")
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "two.txt", "2\n")
	commit(t, dir, "two")
	before := rev(t, bare, "refs/heads/main")
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{ExpectedTip: reviewed.Tip}); err == nil {
		t.Fatal("pushed a branch that moved after review")
	}
	if rev(t, bare, "refs/heads/main") != before {
		t.Fatal("remote changed after a refused push")
	}
	if err := PushBranch(ctx, dir, "main", "origin", "main", PushOptions{ExpectedTip: rev(t, dir, "main")}); err != nil {
		t.Fatal(err)
	}
}
