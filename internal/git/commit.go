package git

import (
	"context"
	"errors"
	"strings"
	"time"
)

func CommitIndex(ctx context.Context, dir, message string) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("enter a commit message")
	}
	if len(message) > 64<<10 || strings.ContainsRune(message, 0) {
		return errors.New("invalid commit message")
	}
	// A commit without paths uses the index and still runs the configured Git hooks.
	_, err := runWithTimeout(ctx, 2*time.Minute, dir, []byte(message), 1<<20, "commit", "-F", "-", "--cleanup=verbatim")
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
