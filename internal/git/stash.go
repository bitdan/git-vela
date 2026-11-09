package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type StashEntry struct {
	Selector string `json:"selector"`
	Hash     string `json:"hash"`
	Subject  string `json:"subject"`
	// Parent is the commit the stash was created on (stash^1), the "before"
	// side of a stash diff view.
	Parent string `json:"parent"`
}

func Stashes(ctx context.Context, dir string) ([]StashEntry, error) {
	data, err := run(ctx, dir, nil, 4<<20, "stash", "list", "--no-color", "-z", "--format=%gd%x00%H%x00%gs%x00%P")
	if err != nil {
		return nil, err
	}
	entries := []StashEntry{}
	if len(data) == 0 {
		return entries, nil
	}
	fields := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	if len(fields)%4 != 0 {
		return nil, errors.New("invalid Git stash record")
	}
	for i := 0; i < len(fields); i += 4 {
		parent := strings.Fields(fields[i+3])
		entry := StashEntry{Selector: fields[i], Hash: fields[i+1], Subject: fields[i+2]}
		if len(parent) != 0 {
			entry.Parent = parent[0]
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func SaveStash(ctx context.Context, dir, message string, untracked bool) error {
	if len(message) > 1024 || strings.ContainsAny(message, "\x00\r\n") {
		return errors.New("the stash message must fit on one line")
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	if _, err := headID(ctx, dir); err != nil {
		return err
	}
	args := []string{"stash", "push"}
	// Stash acts on the current working tree and index and leaves ignored files alone.
	if untracked {
		args = append(args, "--include-untracked")
	}
	if message != "" {
		args = append(args, "--message="+message)
	}
	data, err := runWithTimeout(ctx, time.Minute, dir, nil, 1<<20, args...)
	if err == nil && strings.Contains(string(data), "No local changes to save") {
		return errors.New("no local changes to stash")
	}
	return err
}

func UpdateStash(ctx context.Context, dir, action, selector, hash string, restoreIndex bool) error {
	if action != "apply" && action != "pop" && action != "drop" {
		return errors.New("invalid stash action")
	}
	if !objectID(hash) {
		return errors.New("select a valid stash entry")
	}
	if action == "drop" {
		return dropStash(ctx, dir, selector, hash)
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	if err := requireCleanWorktree(ctx, dir); err != nil {
		return err
	}
	if _, err := findStash(ctx, dir, selector, hash, action == "pop"); err != nil {
		return err
	}
	args := []string{"stash", "apply", "--no-index"}
	if restoreIndex {
		args[2] = "--index"
	}
	_, err := runWithTimeout(ctx, time.Minute, dir, nil, 1<<20, append(args, "--", hash)...)
	if err != nil {
		conflicts, _ := run(ctx, dir, nil, 1<<20, "ls-files", "--unmerged", "-z")
		if len(conflicts) > 0 {
			return fmt.Errorf("stash apply has conflicts; the stash was kept: %w", err)
		}
		return err
	}
	if action == "pop" {
		if err := dropStash(ctx, dir, selector, hash); err != nil {
			return fmt.Errorf("stash applied but removal failed: %w", err)
		}
	}
	return nil
}

func dropStash(ctx context.Context, dir, selector, hash string) error {
	entry, err := findStash(ctx, dir, selector, hash, true)
	if err != nil {
		return err
	}
	_, err = run(ctx, dir, nil, 1<<20, "stash", "drop", "--", entry.Selector)
	return err
}

func findStash(ctx context.Context, dir, selector, hash string, matchSelector bool) (StashEntry, error) {
	entries, err := Stashes(ctx, dir)
	if err != nil {
		return StashEntry{}, err
	}
	for _, entry := range entries {
		if entry.Hash == hash && (!matchSelector || entry.Selector == selector) {
			return entry, nil
		}
	}
	return StashEntry{}, errors.New("the stash list changed; review the refreshed entries and try again")
}
