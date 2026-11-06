package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func headID(ctx context.Context, dir string) (string, error) {
	data, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", errors.New("the repository has no current commit")
		}
		return "", err
	}
	hash := strings.TrimSpace(string(data))
	if !objectID(hash) {
		return "", errors.New("Git returned an invalid HEAD")
	}
	return hash, nil
}

func checkOperationState(ctx context.Context, dir string) error {
	gitDir, err := metadataPath(ctx, dir, "--absolute-git-dir")
	if err != nil {
		return err
	}
	for _, name := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply", "sequencer"} {
		_, err := os.Lstat(filepath.Join(gitDir, name))
		switch {
		case err == nil:
			return fmt.Errorf("finish the Git operation in progress (%s) first", name)
		case !errors.Is(err, os.ErrNotExist):
			return err
		}
	}
	return nil
}

func requireCleanWorktree(ctx context.Context, dir string) error {
	data, err := run(ctx, dir, nil, 1<<20, "status", "--porcelain=v1", "-z", "--untracked-files=normal", "--ignore-submodules=none", "--")
	if err != nil {
		return err
	}
	if len(data) != 0 {
		return errors.New("this operation requires a clean working tree and index; commit or stash your local changes first")
	}
	return nil
}

func headReference(ctx context.Context, dir string) (string, error) {
	data, err := run(ctx, dir, nil, 4096, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
