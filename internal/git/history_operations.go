package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
)

type HeadState struct {
	Hash string `json:"hash"`
	Ref  string `json:"ref"`
}

type OperationState struct {
	Kind      string   `json:"kind"`
	Conflicts []string `json:"conflicts"`
}

func ReadHead(ctx context.Context, dir string) (HeadState, error) {
	hash, err := headID(ctx, dir)
	if err != nil {
		return HeadState{}, err
	}
	ref, err := headReference(ctx, dir)
	return HeadState{Hash: hash, Ref: ref}, err
}

func ReadOperationState(ctx context.Context, dir string) (OperationState, error) {
	state := OperationState{}
	gitDir, err := metadataPath(ctx, dir, "--absolute-git-dir")
	if err != nil {
		return state, err
	}
	for _, marker := range []struct{ file, kind string }{
		{"rebase-merge", "rebase"}, {"rebase-apply", "rebase"}, {"MERGE_HEAD", "merge"},
		{"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"},
	} {
		if _, err := os.Lstat(filepath.Join(gitDir, marker.file)); err == nil {
			state.Kind = marker.kind
			if marker.file == "rebase-apply" {
				if _, err := os.Lstat(filepath.Join(gitDir, marker.file, "applying")); err == nil {
					state.Kind = "am"
				}
			}
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return state, err
		}
	}
	if state.Kind == "" {
		todo, err := os.ReadFile(filepath.Join(gitDir, "sequencer", "todo"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return state, err
		}
		if strings.HasPrefix(string(todo), "pick ") {
			state.Kind = "cherry-pick"
		} else if strings.HasPrefix(string(todo), "revert ") {
			state.Kind = "revert"
		}
	}
	data, err := run(ctx, dir, nil, 8<<20, "diff", "--name-only", "--diff-filter=U", "-z", "--")
	if err != nil {
		return state, err
	}
	state.Conflicts = splitPaths(data)
	return state, nil
}

func ApplyHistoryOperation(ctx context.Context, dir, action, target string, expected HeadState, mode string) error {
	if !objectID(target) {
		return errors.New("select a valid target commit")
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	head, err := ReadHead(ctx, dir)
	if err != nil {
		return err
	}
	if head != expected {
		return errors.New("HEAD or the current branch changed; review the operation again")
	}
	if head.Ref == "" {
		return errors.New("check out a local branch before applying a history operation")
	}
	if _, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", target+"^{commit}"); err != nil {
		return err
	}

	var args []string
	switch action {
	case "reset":
		if mode != "soft" && mode != "mixed" && mode != "hard" {
			return errors.New("select a reset mode")
		}
		state, err := ReadOperationState(ctx, dir)
		if err != nil {
			return err
		}
		if len(state.Conflicts) != 0 {
			return errors.New("resolve the unmerged files before resetting")
		}
		if mode == "hard" {
			if err := checkUntrackedCollisions(ctx, dir, target); err != nil {
				return err
			}
		}
		args = []string{"reset", "--" + mode, "--no-recurse-submodules", target, "--"}
	case "cherry-pick", "revert", "merge":
		if err := requireCleanWorktree(ctx, dir); err != nil {
			return err
		}
		switch action {
		case "cherry-pick", "revert":
			// The current UI handles a single parent without choosing a mainline.
			if _, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", "--quiet", target+"^2"); err == nil {
				return errors.New("this operation requires a non-merge commit")
			}
			collisionTree := target
			if action == "revert" {
				if _, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", "--quiet", target+"^"); err == nil {
					collisionTree = target + "^"
				}
			}
			if err := checkUntrackedCollisions(ctx, dir, collisionTree); err != nil {
				return err
			}
			args = []string{action, "--no-edit", target}
		case "merge":
			args = []string{"-c", "submodule.recurse=false", "merge", "--no-edit", "--no-autostash", "--no-overwrite-ignore", "--", target}
		}
	default:
		return errors.New("invalid history operation")
	}
	_, err = runWithTimeout(ctx, 2*time.Minute, dir, nil, 1<<20, args...)
	return err
}

func FinishHistoryOperation(ctx context.Context, dir, expectedKind string, abort bool) error {
	state, err := ReadOperationState(ctx, dir)
	if err != nil {
		return err
	}
	if state.Kind == "" || state.Kind != expectedKind {
		return errors.New("the operation changed; refresh and review the repository")
	}
	command := "--continue"
	if abort {
		command = "--abort"
	} else if len(state.Conflicts) != 0 {
		return errors.New("resolve and stage all unmerged files before continuing")
	}
	_, err = runWithEnvironment(ctx, 2*time.Minute, dir, nil, 1<<20, []string{"GIT_EDITOR=true"}, state.Kind, command)
	return err
}

func ResolveConflict(ctx context.Context, dir, path string) error {
	if err := validateHistoryPath(path); err != nil {
		return err
	}
	state, err := ReadOperationState(ctx, dir)
	if err != nil {
		return err
	}
	if !slices.Contains(state.Conflicts, path) {
		return errors.New("the selected file is no longer unmerged")
	}
	_, err = run(ctx, dir, nil, 1<<20, "add", "-A", "--", ":(top,literal)"+path)
	return err
}

func splitPaths(data []byte) []string {
	if len(data) == 0 {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
}

func checkUntrackedCollisions(ctx context.Context, dir, target string) error {
	// Without --exclude-standard, this includes ignored files too.
	data, err := run(ctx, dir, nil, 16<<20, "ls-files", "--others", "-z", "--")
	if err != nil || len(data) == 0 {
		return err
	}
	tree, err := run(ctx, dir, nil, 16<<20, "ls-tree", "-r", "--name-only", "-z", target, "--")
	if err != nil {
		return err
	}
	paths := splitPaths(tree)
	key := func(path string) string {
		path = strings.TrimSuffix(path, "/")
		if runtime.GOOS == "windows" {
			return strings.ToLower(path)
		}
		return path
	}
	for i, path := range paths {
		paths[i] = key(path)
	}
	sort.Strings(paths)
	for _, path := range splitPaths(data) {
		name := key(path)
		index := sort.SearchStrings(paths, name)
		if index < len(paths) && paths[index] == name {
			return fmt.Errorf("the target may overwrite the untracked or ignored path %q", path)
		}
		prefix := name + "/"
		index = sort.SearchStrings(paths, prefix)
		if index < len(paths) && strings.HasPrefix(paths[index], prefix) {
			return fmt.Errorf("the target may overwrite the untracked or ignored path %q", path)
		}
		for parent, _, found := strings.Cut(name, "/"); found; {
			if _, exists := slices.BinarySearch(paths, parent); exists {
				return fmt.Errorf("the target may overwrite the untracked or ignored path %q", path)
			}
			next := strings.IndexByte(name[len(parent)+1:], '/')
			if next < 0 {
				break
			}
			parent = name[:len(parent)+1+next]
		}
	}
	return nil
}
