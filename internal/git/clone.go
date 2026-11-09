package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Clone copies a repository from a URL or local path into a new directory.
// The target directory must not exist or must be empty; the parent folder
// must exist so Git can create the target.
func Clone(ctx context.Context, address, dir, branch string) error {
	address = strings.TrimSpace(address)
	if address == "" || strings.ContainsAny(address, "\x00\r\n") ||
		strings.HasPrefix(address, "-") || strings.HasPrefix(address, "--") {
		return errors.New("enter a valid repository URL or path")
	}
	if len(address) > 2048 {
		return errors.New("the repository URL is too long")
	}
	if !filepath.IsAbs(dir) {
		return errors.New("choose an absolute target folder for the clone")
	}
	if strings.TrimSpace(filepath.Base(dir)) == "." || dir == filepath.VolumeName(dir)+string(filepath.Separator) {
		return errors.New("choose a new folder below an existing parent")
	}
	parent := filepath.Dir(dir)
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("the parent folder of the target does not exist")
	}
	if _, err := os.Lstat(dir); err == nil {
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			return readErr
		}
		if len(entries) != 0 {
			return errors.New("the target folder already exists and is not empty")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	branch = strings.TrimSpace(branch)
	if branch != "" {
		if _, err := run(ctx, parent, nil, 4096, "check-ref-format", "refs/heads/"+branch); err != nil {
			return errors.New("enter a valid branch name")
		}
	}
	args := []string{"clone", "--no-progress", "--recurse-submodules=no"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--", address, dir)
	_, err = runWithTimeout(ctx, 10*time.Minute, parent, nil, 1<<20, args...)
	return err
}
