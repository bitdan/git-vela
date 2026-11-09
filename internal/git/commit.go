package git

import (
	"context"
	"errors"
	"strings"
	"time"
)

type AmendTarget struct {
	Ref     string `json:"ref"`
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

func PrepareAmend(ctx context.Context, dir string) (AmendTarget, error) {
	if err := checkOperationState(ctx, dir); err != nil {
		return AmendTarget{}, err
	}
	hash, err := headID(ctx, dir)
	if err != nil {
		return AmendTarget{}, err
	}
	ref, err := headReference(ctx, dir)
	if err != nil {
		return AmendTarget{}, err
	}
	message, err := ReadCommitMessage(ctx, dir, hash)
	return AmendTarget{Hash: hash, Ref: ref, Message: message}, err
}

func AmendIndex(ctx context.Context, dir, expectedHead, expectedRef, message string) error {
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	head, err := headID(ctx, dir)
	if err != nil {
		return err
	}
	ref, err := headReference(ctx, dir)
	if err != nil {
		return err
	}
	if head != expectedHead || ref != expectedRef {
		return errors.New("HEAD changed; reopen the amend form to review the current commit")
	}
	return writeCommit(ctx, dir, message, "--amend")
}

func CommitIndex(ctx context.Context, dir, message string) error {
	return writeCommit(ctx, dir, message)
}

func writeCommit(ctx context.Context, dir, message string, options ...string) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("enter a commit message")
	}
	if len(message) > 64<<10 || strings.ContainsRune(message, 0) {
		return errors.New("invalid commit message")
	}
	// A commit without paths uses the index and still runs the configured Git hooks.
	args := append([]string{"commit", "-F", "-", "--cleanup=verbatim"}, options...)
	_, err := runWithTimeout(ctx, 2*time.Minute, dir, []byte(message), 1<<20, args...)
	return err
}

func ReadCommitMessage(ctx context.Context, dir, hash string) (string, error) {
	if !objectID(hash) {
		return "", errors.New("invalid commit hash")
	}
	data, err := run(ctx, dir, nil, 512<<10, "show", "--no-patch", "--no-notes",
		"--no-show-signature", "--encoding=UTF-8", "--format=format:%B", hash+"^{commit}", "--")
	return string(data), err
}

// FirstParent resolves the first parent of a commit. Squash-into-parent needs
// the grandparent as the rebase base, so the frontend resolves two levels.
func FirstParent(ctx context.Context, dir, hash string) (string, error) {
	if !objectID(hash) {
		return "", errors.New("invalid commit hash")
	}
	data, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", "--quiet", hash+"^")
	if err != nil {
		return "", errors.New("this commit has no parent")
	}
	parent := strings.TrimSpace(string(data))
	if !objectID(parent) {
		return "", errors.New("this commit has no parent")
	}
	return parent, nil
}
