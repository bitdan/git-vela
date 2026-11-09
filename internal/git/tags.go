package git

import (
	"context"
	"errors"
	"strings"
)

// Tag name validation follows the same refname rules as branches (G1).
// Names are stored as refs/tags/<name>; one-level names like "v1.0" and
// nested names like "release/v1" are both accepted.
func checkTagName(ctx context.Context, dir, name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsRune(name, 0) {
		return errors.New("enter a valid tag name")
	}
	if _, err := run(ctx, dir, nil, 4096, "check-ref-format", "refs/tags/"+name); err != nil {
		return err
	}
	return nil
}

// CreateTag adds a lightweight or annotated tag pointing at commit.
// An empty message creates a lightweight tag; annotated tags reject duplicates
// like lightweight ones (git refuses to overwrite an existing tag).
func CreateTag(ctx context.Context, dir, name, commit, message string) error {
	if !objectID(commit) {
		return errors.New("select a commit for the new tag")
	}
	if err := checkTagName(ctx, dir, name); err != nil {
		return err
	}
	if len(message) > 8192 {
		return errors.New("tag messages are limited to 8 KiB")
	}
	args := []string{"tag", "--no-sign"}
	if message != "" {
		args = append(args, "-m", message)
	}
	args = append(args, "--", name, commit+"^{commit}")
	_, err := run(ctx, dir, nil, 8192, args...)
	return err
}

func DeleteTag(ctx context.Context, dir, name string) error {
	if err := checkTagName(ctx, dir, name); err != nil {
		return err
	}
	if _, err := run(ctx, dir, nil, 8192, "show-ref", "--verify", "--quiet", "refs/tags/"+name); err != nil {
		return err
	}
	_, err := run(ctx, dir, nil, 8192, "tag", "-d", "--", name)
	return err
}

// PushTag publishes an existing local tag. Force is intentionally unavailable:
// moving a published tag rewrites shared history pointers.
func PushTag(ctx context.Context, dir, remote, name string) error {
	if err := checkTagName(ctx, dir, name); err != nil {
		return err
	}
	if err := requireRemote(ctx, dir, remote); err != nil {
		return err
	}
	if _, err := run(ctx, dir, nil, 8192, "show-ref", "--verify", "--quiet", "refs/tags/"+name); err != nil {
		return err
	}
	return runRemote(ctx, dir, "push", "--no-force", "--no-mirror", "--no-prune",
		"--recurse-submodules=no", "--", remote, "refs/tags/"+name+":refs/tags/"+name)
}

// CheckoutTag moves HEAD to a detached state at the tagged commit, mirroring
// the existing SwitchBranch contract (git itself refuses a dirty checkout).
func CheckoutTag(ctx context.Context, dir, name string) error {
	if err := checkTagName(ctx, dir, name); err != nil {
		return err
	}
	if _, err := run(ctx, dir, nil, 8192, "show-ref", "--verify", "--quiet", "refs/tags/"+name); err != nil {
		return err
	}
	_, err := run(ctx, dir, nil, 8192, "switch", "--detach", "--no-guess",
		"--no-recurse-submodules", "--", "refs/tags/"+name)
	return err
}
