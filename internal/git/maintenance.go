package git

import (
	"context"
	"strings"
	"time"
)

// RunGC compacts the object database and prunes unreachable objects, which is
// what IDEA's Git → Cleanup action does. It runs unattended and can take a
// while on large repositories, so it gets its own timeout.
func RunGC(ctx context.Context, dir string) error {
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	_, err := runWithTimeout(ctx, 10*time.Minute, dir, nil, 1<<20, "gc")
	return err
}

// UnreachableCommits counts commits no ref reaches, so the cleanup dialog can
// say what `git gc` would prune. It runs the cheap connectivity-only check.
func UnreachableCommits(ctx context.Context, dir string) (int, error) {
	data, err := run(ctx, dir, nil, 64<<10, "fsck", "--unreachable", "--no-reflogs", "--connectivity-only")
	if err != nil {
		return 0, err
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "unreachable commit") {
			count++
		}
	}
	return count, nil
}
