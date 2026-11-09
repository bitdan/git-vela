package git

import (
	"context"
	"errors"
	"strings"
	"time"
)

// RunGC compacts objects and applies Git's configured expiration policies.
// Large repositories may require more time than the standard Git timeout.
func RunGC(ctx context.Context, dir string) error {
	if availability := CheckRepositoryPath(dir); !availability.Available {
		return errors.New(availability.Reason)
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	_, err := runWithTimeout(ctx, 10*time.Minute, dir, nil, 1<<20, "gc")
	return err
}

// UnreachableCommits counts commits unreachable from current refs and the
// index, ignoring reflogs. It is a diagnostic, independent of GC expiration.
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
