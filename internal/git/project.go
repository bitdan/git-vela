package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func Initialize(ctx context.Context, dir string) (Repository, error) {
	if !filepath.IsAbs(dir) {
		return Repository{}, errors.New("choose an existing project folder or enter its absolute path")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return Repository{}, err
	}
	if !info.IsDir() {
		return Repository{}, errors.New("choose a project folder")
	}
	if _, err := run(ctx, dir, nil, 32768, "rev-parse", "--absolute-git-dir"); err == nil {
		return Repository{}, errors.New("this folder is already inside a Git repository; open it instead")
	} else {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || !strings.Contains(err.Error(), "not a git repository") {
			return Repository{}, err
		}
	}
	// A damaged or incomplete .git entry is still repository data.
	if _, err := os.Lstat(filepath.Join(dir, ".git")); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return Repository{}, err
		}
		return Repository{}, errors.New("this folder already contains a .git entry")
	}
	if _, err := run(ctx, dir, nil, 32768, "init", "--initial-branch=main"); err != nil {
		return Repository{}, err
	}
	return Open(ctx, dir)
}

func StageAll(ctx context.Context, dir string) error {
	conflicts, err := run(ctx, dir, nil, 1<<20, "ls-files", "--unmerged", "-z")
	if err != nil {
		return err
	}
	if len(conflicts) != 0 {
		return errors.New("resolve unmerged files before staging the whole working tree")
	}
	_, err = runWithTimeout(ctx, time.Minute, dir, nil, 1<<20, "add", "-A", "--")
	return err
}
