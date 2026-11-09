package git

import (
	"context"
	"errors"
	"fmt"
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
	if outputLine(data) != name {
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

// BranchDeletion describes what deleting a local branch would discard, so
// the confirmation dialog can name it (IDEA GitDeleteBranchOperation detects
// "not fully merged" and offers Restore afterwards).
type BranchDeletion struct {
	Branch string `json:"branch"`
	Tip    string `json:"tip"`
	// Base is the checked-out branch, or HEAD when detached.
	Base     string `json:"base"`
	Unmerged int    `json:"unmerged"`
}

func InspectBranchDeletion(ctx context.Context, dir, name string) (BranchDeletion, error) {
	if err := checkBranchName(ctx, dir, name); err != nil {
		return BranchDeletion{}, err
	}
	headRef, err := headReference(ctx, dir)
	if err != nil {
		return BranchDeletion{}, err
	}
	if headRef == "refs/heads/"+name {
		return BranchDeletion{}, errors.New("check out another branch before deleting the current one")
	}
	tip, err := revision(ctx, dir, "refs/heads/"+name)
	if err != nil {
		return BranchDeletion{}, fmt.Errorf("local branch %s no longer exists; refresh the branch list", name)
	}
	info := BranchDeletion{Branch: name, Tip: tip, Base: "HEAD"}
	if headRef != "" {
		info.Base = strings.TrimPrefix(headRef, "refs/heads/")
	}
	count, err := run(ctx, dir, nil, 4096, "rev-list", "--count", "HEAD.."+tip)
	if err != nil {
		return BranchDeletion{}, err
	}
	info.Unmerged = parseCount(strings.TrimSpace(string(count)))
	return info, nil
}

// DeleteBranch deletes a local branch whose tip still matches the reviewed
// one. A branch with commits outside HEAD is deleted only with force, which
// the UI passes after showing those commits; the tip check keeps a branch
// that moved after the review.
func DeleteBranch(ctx context.Context, dir, name, expectedTip string, force bool) error {
	if !objectID(expectedTip) {
		return errors.New("review the branch before deleting it")
	}
	info, err := InspectBranchDeletion(ctx, dir, name)
	if err != nil {
		return err
	}
	if info.Tip != expectedTip {
		return errors.New("the branch moved after it was reviewed; review it again before deleting")
	}
	if info.Unmerged > 0 && !force {
		return errors.New("the branch contains commits outside the current HEAD")
	}
	if path, err := checkedOutElsewhere(ctx, dir, name); err != nil {
		return err
	} else if path != "" {
		return fmt.Errorf("branch %s is checked out in worktree %s", name, path)
	}
	// update-ref with the old value is an atomic compare-and-delete.
	if _, err := run(ctx, dir, nil, 8192, "update-ref", "-m", "branch: deleted by GitVela", "-d", "refs/heads/"+name, expectedTip); err != nil {
		return errors.New("the branch changed while it was being deleted; refresh and try again")
	}
	_, _ = run(ctx, dir, nil, 8192, "config", "--remove-section", "branch."+name)
	return nil
}
