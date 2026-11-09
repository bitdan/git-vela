package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// BisectState is IDEA's "Log → Bisect" panel data: whether a bisect run is
// active, the marks so far, and the commit Git is currently asking about.
type BisectState struct {
	Active    bool     `json:"active"`
	Bad       string   `json:"bad"`
	Good      []string `json:"good"`
	Current   string   `json:"current"`
	Subject   string   `json:"subject"`
	Remaining int      `json:"remaining"`
	Finished  bool     `json:"finished"`
	FirstBad  string   `json:"firstBad"`
}

func gitDir(ctx context.Context, dir string) (string, error) {
	data, err := run(ctx, dir, nil, 64<<10, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// BisectState reports the current bisect run. `git bisect` keeps its state in
// .git/BISECT_START and refs/bisect/*, which is stable across Git versions;
// the remaining candidates are counted here instead of parsing human output.
func ReadBisectState(ctx context.Context, dir string) (BisectState, error) {
	state := BisectState{Good: []string{}}
	root, err := gitDir(ctx, dir)
	if err != nil {
		return state, err
	}
	if _, err := os.Stat(filepath.Join(root, "BISECT_START")); err != nil {
		return state, nil
	}
	state.Active = true
	if data, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", "--quiet", "refs/bisect/bad"); err == nil {
		state.Bad = strings.TrimSpace(string(data))
	}
	good, err := run(ctx, dir, nil, 1<<20, "for-each-ref", "--format=%(objectname)", "refs/bisect/good-*")
	if err != nil {
		return state, err
	}
	for _, line := range strings.Split(string(good), "\n") {
		if id := strings.TrimSpace(line); objectID(id) {
			state.Good = append(state.Good, id)
		}
	}
	if head, err := ReadHead(ctx, dir); err == nil {
		state.Current = head.Hash
	}
	if state.Current != "" {
		if data, err := run(ctx, dir, nil, 64<<10, "show", "-s", "--format=%s", state.Current); err == nil {
			state.Subject = strings.TrimSpace(string(data))
		}
	}
	if state.Bad != "" && len(state.Good) > 0 {
		remaining, err := countBisectCandidates(ctx, dir, state.Bad, state.Good)
		if err == nil {
			state.Remaining = remaining
			// The bad commit itself is the last candidate: Git has found it.
			state.Finished = remaining <= 1
			if state.Finished {
				state.FirstBad = state.Bad
			}
		}
	}
	return state, nil
}

func countBisectCandidates(ctx context.Context, dir, bad string, good []string) (int, error) {
	args := []string{"rev-list", "--count", bad}
	for _, id := range good {
		args = append(args, "^"+id)
	}
	data, err := run(ctx, dir, nil, 4096, append(args, "--")...)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, field := range strings.Fields(string(data)) {
		if value, err := strconv.Atoi(field); err == nil {
			count += value
		}
	}
	return count, nil
}

// StartBisect begins a run between a known bad and a known good commit.
// Both ends accept any revision Git understands (HEAD, a tag, a hash), because
// the panel pre-fills HEAD as the bad end.
func StartBisect(ctx context.Context, dir, bad, good string) error {
	bad, good = strings.TrimSpace(bad), strings.TrimSpace(good)
	if bad == "" || good == "" {
		return errors.New("enter a bad and a good revision")
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	if err := requireCleanWorktree(ctx, dir); err != nil {
		return err
	}
	badID, err := resolveCommit(ctx, dir, bad)
	if err != nil {
		return err
	}
	goodID, err := resolveCommit(ctx, dir, good)
	if err != nil {
		return err
	}
	if badID == goodID {
		return errors.New("the bad and good commits must differ")
	}
	if _, err := run(ctx, dir, nil, 4096, "merge-base", "--is-ancestor", badID, goodID); err == nil {
		return errors.New("the bad commit is an ancestor of the good commit; swap them")
	}
	if state, err := ReadBisectState(ctx, dir); err == nil && state.Active {
		return errors.New("a bisect run is already active; reset it before starting another")
	}
	// Starting detaches HEAD, which is expected for a bisect run. The two
	// positional revisions are the longest-supported form across Git versions.
	_, err = run(ctx, dir, nil, 1<<20, "bisect", "start", badID, goodID)
	return err
}

// resolveCommit turns a revision expression into a full commit hash.
func resolveCommit(ctx context.Context, dir, revision string) (string, error) {
	data, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", "--quiet", revision+"^{commit}")
	id := strings.TrimSpace(string(data))
	if err != nil || !objectID(id) {
		return "", errors.New(revision + " is not a commit in this repository")
	}
	return id, nil
}

// MarkBisect answers the current question. "good" and "bad" are the only
// verdicts: Git prints the first bad commit when the run finishes.
func MarkBisect(ctx context.Context, dir, verdict string) error {
	if verdict != "good" && verdict != "bad" {
		return errors.New("select good or bad")
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	if err := requireCleanWorktree(ctx, dir); err != nil {
		return err
	}
	state, err := ReadBisectState(ctx, dir)
	if err != nil {
		return err
	}
	if !state.Active {
		return errors.New("no bisect run is active")
	}
	_, err = run(ctx, dir, nil, 1<<20, "bisect", verdict)
	return err
}

// ResetBisect ends the run and restores the branch that was checked out when
// the bisect started.
func ResetBisect(ctx context.Context, dir string) error {
	state, err := ReadBisectState(ctx, dir)
	if err != nil {
		return err
	}
	if !state.Active {
		return errors.New("no bisect run is active")
	}
	_, err = run(ctx, dir, nil, 1<<20, "bisect", "reset")
	return err
}
