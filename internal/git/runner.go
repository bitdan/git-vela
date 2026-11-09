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
	"sync"
	"time"
)

var errLimit = errors.New("Git output exceeded the preview limit")

/** The Git binary can be overridden from Settings (Git → Executable). The
 *  process-wide value defaults to PATH lookup and is validated before use. */
var (
	executableMu    sync.RWMutex
	gitExecutable   = "git"
)

func executable() string {
	executableMu.RLock()
	defer executableMu.RUnlock()
	return gitExecutable
}

// ConfigureExecutable validates and applies a Git binary path. An empty path
// restores the PATH lookup; a failing binary keeps the previous value.
func ConfigureExecutable(ctx context.Context, path string) (string, error) {
	candidate := strings.TrimSpace(path)
	if candidate == "" {
		candidate = "git"
	}
	if strings.ContainsAny(candidate, "\x00\n\r") {
		return "", errors.New("invalid Git executable path")
	}
	executableMu.Lock()
	previous := gitExecutable
	gitExecutable = candidate
	executableMu.Unlock()
	version, err := Version(ctx)
	if err != nil {
		executableMu.Lock()
		gitExecutable = previous
		executableMu.Unlock()
		return "", errors.New("could not run the selected Git executable: " + candidate)
	}
	return version, nil
}

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
	cmd := exec.CommandContext(ctx, executable(), append([]string{"--no-pager", "--no-replace-objects"}, args...)...)
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

type RepositoryAvailability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

// CheckRepositoryPath reports whether an open repository folder still exists
// and still holds Git metadata. Background polling stops once the folder is
// deleted, emptied, or its drive disappears: every Git command would fail with
// the same OS-level error and repeat the alert every few seconds.
func CheckRepositoryPath(path string) RepositoryAvailability {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return RepositoryAvailability{Reason: "The repository folder is no longer available: " + path +
			". Restore it, then open the repository again."}
	}
	// Worktrees and submodules use a .git file instead of a folder.
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return RepositoryAvailability{Reason: "The repository folder no longer contains a Git repository: " + path +
			". Restore it, then open the repository again."}
	}
	return RepositoryAvailability{Available: true}
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
