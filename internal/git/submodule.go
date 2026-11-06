package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func SubmodulePath(ctx context.Context, dir, path string) (string, error) {
	if err := validateHistoryPath(path); err != nil {
		return "", err
	}
	repo, err := Open(ctx, dir)
	if err != nil {
		return "", err
	}
	data, err := run(ctx, repo.Path, nil, 8192, "ls-files", "--stage", "-z", "--", ":(top,literal)"+path)
	if err != nil {
		return "", err
	}
	tracked := false
	for _, record := range splitPaths(data) {
		metadata, name, _ := strings.Cut(record, "\t")
		if name == path && strings.HasPrefix(metadata, "160000 ") {
			tracked = true
			break
		}
	}
	if !tracked {
		return "", errors.New("the selected path is not a submodule in the current checkout")
	}
	local, err := filepath.Localize(path)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(repo.Path)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(local)
	if err != nil || !info.IsDir() {
		return "", errors.New("initialize the submodule checkout before opening it")
	}
	submodule, err := Open(ctx, filepath.Join(repo.Path, local))
	if err != nil {
		return "", err
	}
	subInfo, err := os.Stat(submodule.Path)
	if err != nil || !os.SameFile(info, subInfo) {
		return "", errors.New("initialize the submodule checkout before opening it")
	}
	return submodule.Path, nil
}
