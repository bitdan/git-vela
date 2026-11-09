package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RebaseStep struct {
	Hash    string `json:"hash"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
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
	// Reword and squash-with-message are expressed as a pick/fixup followed by
	// "exec git commit --amend -F <file>" so no editor can stall the rebase.
	var messages []string
	for _, step := range steps {
		if !remaining[step.Hash] {
			return errors.New("the plan must contain each commit exactly once")
		}
		delete(remaining, step.Hash)
		switch step.Action {
		case "pick":
			hasPick = true
			plan.WriteString("pick " + step.Hash + "\n")
		case "reword":
			hasPick = true
			message, err := rebaseMessage(step.Message)
			if err != nil {
				return err
			}
			messages = append(messages, message)
			plan.WriteString("pick " + step.Hash + "\n")
			plan.WriteString(rebaseAmendExec(len(messages)) + "\n")
		case "squash", "fixup":
			if !hasPick {
				return errors.New("the first retained commit must be picked")
			}
			if step.Action == "squash" && strings.TrimSpace(step.Message) != "" {
				message, err := rebaseMessage(step.Message)
				if err != nil {
					return err
				}
				messages = append(messages, message)
				plan.WriteString("fixup " + step.Hash + "\n")
				plan.WriteString(rebaseAmendExec(len(messages)) + "\n")
			} else {
				plan.WriteString(step.Action + " " + step.Hash + "\n")
			}
		case "edit":
			hasPick = true
			plan.WriteString("edit " + step.Hash + "\n")
		case "drop":
			plan.WriteString("drop " + step.Hash + "\n")
		default:
			return errors.New("invalid rebase action")
		}
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
	if len(messages) != 0 {
		messagesDir, err := writeRebaseMessages(messages)
		if err != nil {
			return err
		}
		defer os.RemoveAll(messagesDir)
		env = append(env, "GITVELA_REBASE_MSGS="+filepath.ToSlash(messagesDir))
	}
	_, err = runWithEnvironment(ctx, 2*time.Minute, dir, nil, 1<<20, env, "-c", "submodule.recurse=false", "rebase",
		"--interactive", "--no-autosquash", "--no-autostash", "--no-update-refs", "--no-rebase-merges",
		"--no-fork-point", "--reapply-cherry-picks", "--empty=drop", "--", base)
	return err
}

func rebaseAmendExec(index int) string {
	// --allow-empty keeps rewording an empty commit legal; it never makes a
	// non-empty commit empty.
	return "exec git commit --amend --allow-empty --no-verify -F \"$GITVELA_REBASE_MSGS/" + strconv.Itoa(index) + ".msg\""
}

func rebaseMessage(message string) (string, error) {
	message = strings.Trim(message, "\r\n")
	if message == "" || strings.ContainsAny(message, "\x00") {
		return "", errors.New("enter a commit message for the reworded commit")
	}
	if len(message) > 1<<20 {
		return "", errors.New("the commit message is too long")
	}
	return message, nil
}

func writeRebaseMessages(messages []string) (string, error) {
	messagesDir, err := os.MkdirTemp("", "gitvela-rebase-msgs-")
	if err != nil {
		return "", err
	}
	for index, message := range messages {
		path := filepath.Join(messagesDir, strconv.Itoa(index+1)+".msg")
		if err := os.WriteFile(path, []byte(message), 0o600); err != nil {
			os.RemoveAll(messagesDir)
			return "", err
		}
	}
	return messagesDir, nil
}
