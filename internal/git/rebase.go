package git

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type RebaseStep struct {
	Hash   string `json:"hash"`
	Action string `json:"action"`
}

func ReadRebasePlan(ctx context.Context, dir, base string, expected HeadState) ([]Commit, error) {
	if !objectID(base) || !objectID(expected.Hash) || expected.Ref == "" {
		return nil, errors.New("select a target and check out a local branch before rebasing")
	}
	head, err := ReadHead(ctx, dir)
	if err != nil {
		return nil, err
	}
	if head != expected {
		return nil, errors.New("HEAD or the current branch changed; build a new rebase plan")
	}
	data, err := run(ctx, dir, nil, 8<<20, "log", "--reverse", "--topo-order", "--no-show-signature", "--no-notes",
		"--encoding=UTF-8", "-z", "--format=%H%x00%P%x00%an%x00%at%x00%s", "-n", strconv.Itoa(pageSize+1),
		base+"^{commit}.."+head.Hash, "--")
	if err != nil {
		return nil, err
	}
	commits, more, err := parseCommitPage(data, nil)
	if err != nil {
		return nil, err
	}
	if more || len(commits) == 0 {
		return nil, errors.New("interactive rebase requires 1 to 200 commits on the current branch")
	}
	for _, commit := range commits {
		if len(commit.Parents) > 1 {
			return nil, errors.New("interactive rebase requires a linear commit range")
		}
	}
	return commits, nil
}

func RunInteractiveRebase(ctx context.Context, dir, base string, expected HeadState, steps []RebaseStep) error {
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	if err := requireCleanWorktree(ctx, dir); err != nil {
		return err
	}
	commits, err := ReadRebasePlan(ctx, dir, base, expected)
	if err != nil {
		return err
	}
	if len(commits) != len(steps) {
		return errors.New("the plan must contain each commit exactly once")
	}
	remaining := make(map[string]bool, len(commits))
	for _, commit := range commits {
		remaining[commit.Hash] = true
	}
	var plan strings.Builder
	hasPick := false
	for _, step := range steps {
		if !remaining[step.Hash] {
			return errors.New("the plan must contain each commit exactly once")
		}
		delete(remaining, step.Hash)
		switch step.Action {
		case "pick":
			hasPick = true
		case "drop":
		case "squash", "fixup":
			if !hasPick {
				return errors.New("the first retained commit must be picked")
			}
		default:
			return errors.New("invalid rebase action")
		}
		plan.WriteString(step.Action + " " + step.Hash + "\n")
	}
	if err := checkUntrackedCollisions(ctx, dir, base); err != nil {
		return err
	}
	todo, err := os.CreateTemp("", "gitvela-rebase-*.todo")
	if err != nil {
		return err
	}
	defer os.Remove(todo.Name())
	_, writeErr := todo.WriteString(plan.String())
	if closeErr := todo.Close(); closeErr != nil && writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return writeErr
	}

	// The shell command is constant. It reads the validated todo from a quoted
	// environment path and receives Git's own todo path as a positional argument.
	env := []string{
		`GIT_SEQUENCE_EDITOR=f() { cat -- "$GITVELA_REBASE_TODO" > "$1"; }; f`,
		"GITVELA_REBASE_TODO=" + todo.Name(),
		"GIT_EDITOR=true",
	}
	_, err = runWithEnvironment(ctx, 2*time.Minute, dir, nil, 1<<20, env, "-c", "submodule.recurse=false", "rebase",
		"--interactive", "--no-autosquash", "--no-autostash", "--no-update-refs", "--no-rebase-merges",
		"--no-fork-point", "--reapply-cherry-picks", "--empty=drop", "--", base)
	return err
}
