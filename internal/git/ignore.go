package git

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// AddToGitignore appends an anchored pattern for path to the repository-root
// .gitignore, mirroring IDEA's "Add to .gitignore" action for untracked files.
// It reports whether a new entry was written; an existing identical pattern is
// left untouched.
func AddToGitignore(ctx context.Context, dir, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	repo, err := Open(ctx, dir)
	if err != nil {
		return false, err
	}
	pattern, err := gitignorePattern(path)
	if err != nil {
		return false, err
	}
	root, err := os.OpenRoot(repo.Path)
	if err != nil {
		return false, err
	}
	defer root.Close()
	data, err := root.ReadFile(".gitignore")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	content := string(data)
	if hasGitignoreLine(content, pattern) {
		return false, nil
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += pattern + "\n"
	file, err := root.OpenFile(".gitignore", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return false, err
	}
	_, writeErr := file.WriteString(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return false, err
	}
	return true, ctx.Err()
}

// gitignorePattern turns a repository-relative path into an anchored
// .gitignore pattern: forward slashes, no trailing slash, rooted at the
// .gitignore directory. Paths that could escape the repository are rejected.
func gitignorePattern(path string) (string, error) {
	pattern := filepath.ToSlash(strings.TrimSpace(path))
	if pattern == "" || strings.HasPrefix(pattern, "/") {
		return "", errors.New("select a repository file to ignore")
	}
	segments := strings.Split(pattern, "/")
	if segments[0] == ".git" {
		return "", errors.New("select a repository file to ignore")
	}
	for _, segment := range strings.Split(pattern, "/") {
		switch segment {
		case "", ".", "..":
			return "", errors.New("select a repository file to ignore")
		}
	}
	return "/" + pattern, nil
}

func hasGitignoreLine(content, pattern string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == pattern {
			return true
		}
	}
	return false
}
