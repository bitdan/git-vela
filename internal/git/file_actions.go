package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

func ReadFileRevision(ctx context.Context, dir, path, revision string, compareWorking bool) (FilePreview, error) {
	if err := validateHistoryPath(path); err != nil {
		return FilePreview{}, err
	}
	if revision == "" || strings.HasPrefix(revision, "-") {
		return FilePreview{}, errors.New("select a valid Git revision")
	}
	id, err := resolveCommit(ctx, dir, revision)
	if err != nil {
		return FilePreview{}, err
	}
	args := []string{"cat-file", "blob", id + ":" + path}
	if compareWorking {
		args = []string{"--literal-pathspecs", "diff", "--patch", "--no-color", "--no-ext-diff", "--no-textconv", id, "--", path}
	}
	data, err := run(ctx, dir, nil, previewBytes, args...)
	if errors.Is(err, errLimit) || bytes.Count(data, []byte{'\n'}) > previewLines {
		return FilePreview{Kind: "large"}, nil
	}
	if err != nil {
		return FilePreview{}, err
	}
	if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
		return FilePreview{Kind: "binary"}, nil
	}
	return FilePreview{Kind: "text", Content: string(data)}, nil
}

func FileSelectionHistory(ctx context.Context, dir, path string, start, end int) (FileHistoryPage, error) {
	if err := validateHistoryPath(path); err != nil {
		return FileHistoryPage{}, err
	}
	if start < 1 || end < start || end > previewLines {
		return FileHistoryPage{}, errors.New("select a valid range of lines")
	}
	// The editor's line numbers refer to the checked-out version. Git -L
	// interprets them against HEAD, so reject working or staged changes.
	status, err := run(ctx, dir, nil, 64<<10, "--literal-pathspecs", "status", "--porcelain=v1", "-z", "--", path)
	if err != nil {
		return FileHistoryPage{}, err
	}
	if len(status) != 0 {
		return FileHistoryPage{}, errors.New("selection history requires a file that matches HEAD")
	}
	rangeSpec := fmt.Sprintf("%d,%d:%s", start, end, path)
	data, err := run(ctx, dir, nil, 8<<20, "log", "--no-patch", "--no-show-signature", "--no-notes",
		"--encoding=UTF-8", "-z", "--format=%H%x00%P%x00%an%x00%at%x00%s",
		"-n", strconv.Itoa(pageSize+1), "-L", rangeSpec, "HEAD")
	if err != nil {
		return FileHistoryPage{}, err
	}
	commits, more, err := parseCommitPage(data, nil)
	return FileHistoryPage{Commits: commits, More: more}, err
}

// PrepareFileMutation fingerprints the reviewed HEAD, working diff and index
// diff. The mutation rechecks the fingerprint under the service's write lock.
func PrepareFileMutation(ctx context.Context, dir, path, action string) (string, error) {
	if err := validateHistoryPath(path); err != nil {
		return "", err
	}
	if availability := CheckRepositoryPath(dir); !availability.Available {
		return "", errors.New(availability.Reason)
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return "", err
	}
	file, err := findWorkingFile(ctx, dir, path)
	if err != nil {
		return "", err
	}
	allowed := " MD"
	if action == "commit" {
		allowed = " AMD"
	} else if action != "rollback" {
		return "", errors.New("invalid file action")
	}
	if strings.Trim(file.Status, allowed) != "" || (!file.CanStage && !file.CanUnstage) {
		return "", errors.New("this file state does not support the selected action")
	}
	head, err := ReadHead(ctx, dir)
	if err != nil {
		return "", err
	}
	index, err := run(ctx, dir, nil, 64<<10, "--literal-pathspecs", "ls-files", "--stage", "-z", "--", path)
	if err != nil {
		return "", err
	}
	for _, entry := range bytes.Split(index, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		meta, name, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok || string(name) != path || bytes.HasPrefix(meta, []byte("160000 ")) {
			return "", errors.New("select a regular file outside submodules")
		}
	}
	tree, err := run(ctx, dir, nil, 64<<10, "--literal-pathspecs", "ls-tree", "-z", head.Hash, "--", path)
	if err != nil {
		return "", err
	}
	if bytes.HasPrefix(tree, []byte("160000 ")) || bytes.HasPrefix(tree, []byte("040000 ")) {
		return "", errors.New("select a regular file outside submodules")
	}
	digest := sha256.New()
	fmt.Fprintf(digest, "%s\x00%s\x00%s\x00%s\x00%s\x00", path, action, head.Hash, head.Ref, file.Status)
	for _, side := range []string{head.Hash, "--cached"} {
		data, err := run(ctx, dir, nil, expandedBytes, "--literal-pathspecs", "diff",
			"--binary", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", side, "--", path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(digest, "%d\x00", len(data))
		digest.Write(data)
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func ApplyFileMutation(ctx context.Context, dir, path, action, expected, message string) error {
	current, err := PrepareFileMutation(ctx, dir, path, action)
	if err != nil {
		return err
	}
	if current != expected {
		return errors.New("the file or HEAD changed; reopen the action and review the current changes")
	}
	literal := ":(top,literal)" + path
	if action == "commit" {
		return writeCommit(ctx, dir, message, "--only", "--", literal)
	}
	_, err = run(ctx, dir, nil, 1<<20, "restore", "--source=HEAD", "--staged", "--worktree", "--", literal)
	return err
}
