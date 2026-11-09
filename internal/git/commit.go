package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type AmendTarget struct {
	Ref     string `json:"ref"`
	Hash    string `json:"hash"`
	Message string `json:"message"`
	// The author of the commit being amended. `git commit --amend` keeps it
	// unless ResetAuthor is requested, so the form must show what it preserves.
	Author      string `json:"author"`
	AuthorEmail string `json:"authorEmail"`
	AuthorDate  int64  `json:"authorDate"`
}

// AmendOptions mirrors the checkboxes of the amend form. Both default to off:
// amending keeps the original author and adds no trailer.
type AmendOptions struct {
	ResetAuthor bool `json:"resetAuthor"`
	SignOff     bool `json:"signOff"`
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
	if err != nil {
		return AmendTarget{}, err
	}
	target := AmendTarget{Hash: hash, Ref: ref, Message: message}
	data, err := run(ctx, dir, nil, 64<<10, "show", "--no-patch", "--no-notes", "--no-show-signature",
		"--format=%an%x00%ae%x00%at", hash+"^{commit}", "--")
	if err != nil {
		return AmendTarget{}, err
	}
	fields := strings.Split(strings.TrimRight(string(data), "\n"), "\x00")
	if len(fields) == 3 {
		target.Author, target.AuthorEmail = fields[0], fields[1]
		if seconds, err := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64); err == nil {
			target.AuthorDate = seconds
		}
	}
	return target, nil
}

func AmendIndex(ctx context.Context, dir, expectedHead, expectedRef, message string, options AmendOptions) error {
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
	args := []string{"--amend"}
	if options.ResetAuthor {
		// Also resets the author date, which is what "reset author" means in Git.
		args = append(args, "--reset-author")
	}
	if options.SignOff {
		args = append(args, "--signoff")
	}
	return writeCommit(ctx, dir, message, args...)
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

// CommitTemplate reads commit.template (G14) so a new commit starts from the
// repository's template. An unset, missing or relative-to-root path is handled
// the way Git does; an unreadable template must not block committing.
func CommitTemplate(ctx context.Context, dir string) (string, error) {
	data, err := run(ctx, dir, nil, 64<<10, "config", "--get", "commit.template")
	if err != nil {
		// Exit code 1 means the key is unset; that is not an error here.
		return "", nil
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", nil
	}
	path := value
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", nil
		}
		path = filepath.Join(home, strings.TrimLeft(path[1:], `/\`))
	}
	if !filepath.IsAbs(path) {
		root, err := run(ctx, dir, nil, 64<<10, "rev-parse", "--show-toplevel")
		if err != nil {
			return "", nil
		}
		path = filepath.Join(strings.TrimSpace(string(root)), path)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", nil
	}
	if info.Size() > 256<<10 {
		return "", errors.New("commit template is too large")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	return string(content), nil
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
