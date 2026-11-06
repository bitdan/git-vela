package git

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const maxRefWatches = 256

// WatchRepository reports possible changes. Callers still verify with Git and
// poll for edits in nested working directories or on unsupported filesystems.
func WatchRepository(ctx context.Context, root string, changed func()) (func(), error) {
	gitDir, err := metadataPath(ctx, root, "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	commonDir, err := metadataPath(ctx, root, "--git-common-dir")
	if err != nil {
		return nil, err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{root, gitDir, commonDir} {
		if err := watcher.Add(dir); err != nil {
			_ = watcher.Close()
			return nil, err
		}
	}

	refs := filepath.Join(commonDir, "refs")
	worktrees := filepath.Join(commonDir, "worktrees")
	addOptional := func(dir string) {
		if err := watcher.Add(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("Git directory watch unavailable", "path", dir, "error", err)
		}
	}
	addRefs := func(dir string) {
		count := len(watcher.WatchList())
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if count >= maxRefWatches {
				return fs.SkipAll
			}
			if err == nil && entry.IsDir() {
				addOptional(path)
				count++
			}
			return nil
		})
	}
	addRefs(refs)
	addOptional(worktrees)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer watcher.Close()
		var debounce *time.Timer
		var ready <-chan time.Time
		defer func() {
			if debounce != nil {
				debounce.Stop()
			}
		}()
		schedule := func() {
			if debounce == nil {
				debounce = time.NewTimer(180 * time.Millisecond)
			} else {
				debounce.Reset(180 * time.Millisecond)
			}
			ready = debounce.C
		}
		for {
			select {
			case <-stop:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				inRefs := event.Name == refs || strings.HasPrefix(event.Name, refs+string(filepath.Separator))
				if event.Has(fsnotify.Create) {
					if inRefs {
						addRefs(event.Name)
					} else if event.Name == worktrees {
						addOptional(worktrees)
					}
				}
				parent := filepath.Dir(event.Name)
				switch {
				case parent == root || inRefs || parent == worktrees || event.Name == worktrees:
					schedule()
				case parent == gitDir || parent == commonDir:
					switch filepath.Base(event.Name) {
					case "HEAD", "index", "packed-refs", "config", "config.worktree", "shallow", "reftable", "MERGE_HEAD", "rebase-merge", "rebase-apply":
						schedule()
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Warn("Git watch event failed", "error", err)
				schedule()
			case <-ready:
				ready = nil
				changed()
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}, nil
}

func metadataPath(ctx context.Context, root, flag string) (string, error) {
	path, err := run(ctx, root, nil, 32768, "rev-parse", "--path-format=absolute", flag)
	return filepath.Clean(strings.TrimSuffix(strings.TrimSuffix(string(path), "\n"), "\r")), err
}
