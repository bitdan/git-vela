package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Worktree struct {
	Path     string `json:"path"`
	Head     string `json:"head"`
	Branch   string `json:"branch"`
	Main     bool   `json:"main"`
	Bare     bool   `json:"bare"`
	Detached bool   `json:"detached"`
	Locked   bool   `json:"locked"`
	Prunable bool   `json:"prunable"`
}

func Worktrees(ctx context.Context, dir string) ([]Worktree, error) {
	data, err := run(ctx, dir, nil, 2<<20, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var trees []Worktree
	var tree Worktree
	for _, field := range strings.Split(string(data), "\x00") {
		key, value, _ := strings.Cut(field, " ")
		switch key {
		case "":
			if tree.Path != "" {
				tree.Main = len(trees) == 0
				trees = append(trees, tree)
				tree = Worktree{}
			}
		case "worktree":
			tree.Path = filepath.Clean(value)
		case "HEAD":
			tree.Head = value
		case "branch":
			tree.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			tree.Bare = true
		case "detached":
			tree.Detached = true
		case "locked":
			tree.Locked = true
		case "prunable":
			tree.Prunable = true
		}
	}
	return trees, nil
}

// AddWorktree checks out a local branch, or creates it at the supplied commit.
func AddWorktree(ctx context.Context, dir, target, branch, commit string) error {
	if !filepath.IsAbs(target) {
		return errors.New("enter an absolute path for the new worktree directory")
	}
	target = filepath.Clean(target)
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return errors.New("the worktree directory already exists")
	}
	if err := checkBranchName(ctx, dir, branch); err != nil {
		return err
	}

	args := []string{"worktree", "add", "--no-guess-remote"}
	if commit != "" {
		if !objectID(commit) {
			return errors.New("select a commit for the new branch")
		}
		args = append(args, "--no-track", "-b", branch, "--", target, commit+"^{commit}")
	} else {
		if _, err := run(ctx, dir, nil, 8192, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
			return err
		}
		args = append(args, "--", target, branch)
	}
	_, err := runWithTimeout(ctx, time.Minute, dir, nil, 1<<20, args...)
	return err
}

func RemoveWorktree(ctx context.Context, dir, target string) error {
	trees, err := Worktrees(ctx, dir)
	if err != nil {
		return err
	}
	repo, err := Open(ctx, dir)
	if err != nil {
		return err
	}
	active, err := os.Stat(repo.Path)
	if err != nil {
		return err
	}
	for _, tree := range trees {
		if tree.Path != target {
			continue
		}
		if tree.Main || tree.Bare || tree.Locked || tree.Prunable || tree.Detached || tree.Branch == "" {
			return errors.New("only unlocked linked worktrees on a local branch can be removed")
		}
		info, err := os.Stat(tree.Path)
		if err != nil {
			return err
		}
		if os.SameFile(active, info) {
			return errors.New("switch to another worktree before removing this one")
		}
		// Include ignored files, which may contain local configuration or data.
		status, err := run(ctx, tree.Path, nil, 2<<20, "status", "--porcelain=v1", "-z",
			"--untracked-files=normal", "--ignored", "--ignore-submodules=none")
		if err != nil {
			return err
		}
		if len(status) != 0 {
			return errors.New("the worktree contains local changes, untracked files, or ignored files")
		}
		_, err = runWithTimeout(ctx, time.Minute, repo.Path, nil, 8192, "worktree", "remove", "--", tree.Path)
		return err
	}
	return errors.New("the directory is not a registered worktree of this repository")
}
