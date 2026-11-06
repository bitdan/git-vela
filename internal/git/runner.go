package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var errLimit = errors.New("Git output exceeded the preview limit")

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		b.exceeded = true
		return 0, errLimit
	}
	return b.buffer.Write(p)
}

func run(ctx context.Context, dir string, input []byte, limit int, args ...string) ([]byte, error) {
	return runWithTimeout(ctx, 20*time.Second, dir, input, limit, args...)
}

func runWithTimeout(ctx context.Context, timeout time.Duration, dir string, input []byte, limit int, args ...string) ([]byte, error) {
	return runWithEnvironment(ctx, timeout, dir, input, limit, nil, args...)
}

func runWithEnvironment(ctx context.Context, timeout time.Duration, dir string, input []byte, limit int, environment []string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "--no-replace-objects"}, args...)...)
	configureProcess(cmd)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	for _, entry := range cmd.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_SHALLOW_FILE":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1", "LC_ALL=C")
	cmd.Env = append(cmd.Env, environment...)
	cmd.Stdin = bytes.NewReader(input)
	out := &limitedBuffer{limit: limit}
	stderr := &limitedBuffer{limit: 8192}
	cmd.Stdout, cmd.Stderr = out, stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if out.exceeded || stderr.exceeded {
			return nil, errLimit
		}
		return nil, fmt.Errorf("git %s: %w %s", args[0], err, strings.TrimSpace(stderr.buffer.String()))
	}
	return out.buffer.Bytes(), nil
}

func Version(ctx context.Context) (string, error) {
	data, err := run(ctx, "", nil, 4096, "--version")
	return strings.TrimSpace(string(data)), err
}

type Repository struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Head string `json:"head"`
}

func Open(ctx context.Context, path string) (Repository, error) {
	if path == "" {
		return Repository{}, errors.New("select a repository directory")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return Repository{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Repository{}, err
	}
	if !info.IsDir() {
		return Repository{}, errors.New("select a directory, not a file")
	}
	data, err := run(ctx, path, nil, 32768, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return Repository{}, err
	}
	root := filepath.Clean(strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"))
	data, err = run(ctx, root, nil, 32768, "rev-parse", "--abbrev-ref", "HEAD")
	head := strings.TrimSpace(string(data))
	if err != nil {
		data, err = run(ctx, root, nil, 32768, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			return Repository{}, err
		}
		head = strings.TrimSpace(string(data))
	}
	return Repository{Path: root, Name: filepath.Base(root), Head: head}, nil
}
