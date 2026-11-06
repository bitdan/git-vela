package git

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

func checkBranchName(ctx context.Context, dir, name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsRune(name, 0) {
		return errors.New("enter a valid local branch name")
	}
	data, err := run(ctx, dir, nil, 4096, "check-ref-format", "--branch", name)
	if err != nil {
		return err
	}
	if strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r") != name {
		return errors.New("enter an explicit local branch name")
	}
	return nil
}

func CreateBranch(ctx context.Context, dir, name, commit string) error {
	if !objectID(commit) {
		return errors.New("select a commit for the new branch")
	}
	if err := checkBranchName(ctx, dir, name); err != nil {
		return err
	}
	_, err := run(ctx, dir, nil, 8192, "branch", "--no-track", "--", name, commit+"^{commit}")
	return err
}

func SwitchBranch(ctx context.Context, dir, name string) error {
	if err := checkBranchName(ctx, dir, name); err != nil {
		return err
	}
	if _, err := run(ctx, dir, nil, 8192, "show-ref", "--verify", "--quiet", "refs/heads/"+name); err != nil {
		return err
	}
	_, err := run(ctx, dir, nil, 8192, "switch", "--no-guess", "--no-recurse-submodules", "--", name)
	return err
}

func RenameBranch(ctx context.Context, dir, oldName, newName string) error {
	for _, name := range []string{oldName, newName} {
		if err := checkBranchName(ctx, dir, name); err != nil {
			return err
		}
	}
	_, err := run(ctx, dir, nil, 8192, "branch", "-m", "--", oldName, newName)
	return err
}

func DeleteBranch(ctx context.Context, dir, name string) error {
	if err := checkBranchName(ctx, dir, name); err != nil {
		return err
	}
	if _, err := run(ctx, dir, nil, 8192, "merge-base", "--is-ancestor", "refs/heads/"+name, "HEAD"); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return errors.New("the branch contains commits outside the current HEAD")
		}
		return err
	}
	_, err := run(ctx, dir, nil, 8192, "branch", "-d", "--", name)
	return err
}
