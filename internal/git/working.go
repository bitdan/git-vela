package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type WorkingFile struct {
	Path       string `json:"path"`
	OldPath    string `json:"oldPath"`
	Status     string `json:"status"`
	CanStage   bool   `json:"canStage"`
	CanUnstage bool   `json:"canUnstage"`
}

type FilePreview struct {
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

func WorkingChanges(ctx context.Context, dir string) ([]WorkingFile, error) {
	data, err := run(ctx, dir, nil, 16<<20, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--")
	if err != nil {
		return nil, err
	}
	files := []WorkingFile{}
	for len(data) > 0 {
		record, rest, ok := bytes.Cut(data, []byte{0})
		if !ok || len(record) < 4 || record[2] != ' ' {
			return nil, errors.New("invalid Git status record")
		}
		file := WorkingFile{Path: string(record[3:]), Status: string(record[:2])}
		data = rest
		if strings.ContainsAny(file.Status, "RC") {
			old, rest, ok := bytes.Cut(data, []byte{0})
			if !ok || len(old) == 0 {
				return nil, errors.New("missing original Git status path")
			}
			file.OldPath = string(old)
			data = rest
		}
		if !strings.Contains(file.Status, "U") && file.Status != "AA" && file.Status != "DD" {
			file.CanStage = file.Status[1] != ' '
			file.CanUnstage = file.Status[0] != ' ' && file.Status != "??"
		}
		files = append(files, file)
	}
	return files, nil
}

func findWorkingFile(ctx context.Context, dir, path string) (WorkingFile, error) {
	if _, err := filepath.Localize(path); err != nil || path == "." {
		return WorkingFile{}, errors.New("select one changed file")
	}
	files, err := WorkingChanges(ctx, dir)
	if err != nil {
		return WorkingFile{}, err
	}
	for _, file := range files {
		if file.Path == path {
			return file, nil
		}
	}
	return WorkingFile{}, errors.New("the selected file is no longer listed as changed")
}

func workingPathspec(file WorkingFile, side byte) []string {
	paths := []string{":(top,literal)" + file.Path}
	if (side == 'R' || side == 'C') && file.OldPath != "" {
		paths = append(paths, ":(top,literal)"+file.OldPath)
	}
	return paths
}

func SetStaged(ctx context.Context, dir, path string, staged bool) error {
	file, err := findWorkingFile(ctx, dir, path)
	if err != nil {
		return err
	}
	if staged && !file.CanStage || !staged && !file.CanUnstage {
		return errors.New("the selected file no longer supports this index operation")
	}
	args := []string{"reset", "-q", "--"}
	side := file.Status[0]
	if staged {
		args = []string{"add", "-A", "--"}
		side = file.Status[1]
	}
	// A copy leaves its original path untouched.
	if side == 'C' {
		side = ' '
	}
	_, err = run(ctx, dir, nil, 1<<20, append(args, workingPathspec(file, side)...)...)
	return err
}

func WorkingDiff(ctx context.Context, dir, path string, staged bool) (FilePreview, error) {
	file, err := findWorkingFile(ctx, dir, path)
	if err != nil {
		return FilePreview{}, err
	}
	args := []string{"diff", "--patch", "--no-ext-diff", "--no-textconv", "--no-color", "--no-relative"}
	side := file.Status[1]
	if staged {
		args = append(args, "--cached")
		side = file.Status[0]
	}
	args = append(args, "--")
	data, err := run(ctx, dir, nil, previewBytes, append(args, workingPathspec(file, side)...)...)
	if errors.Is(err, errLimit) || bytes.Count(data, []byte{'\n'}) > previewLines {
		return FilePreview{Kind: "large"}, nil
	}
	if err != nil {
		return FilePreview{}, err
	}
	return FilePreview{Kind: "text", Content: string(data)}, nil
}
func WorkingPreview(dir, path string) (FilePreview, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return FilePreview{}, err
	}
	defer root.Close()
	preview, _, err := readWorkingPreview(root, path)
	return preview, err
}

func readWorkingPreview(root *os.Root, path string) (FilePreview, os.FileInfo, error) {
	local, err := filepath.Localize(path)
	if err != nil || local == "." {
		return FilePreview{}, nil, errors.New("invalid working file path")
	}

	// Walk without following symlinks or exposing repository metadata.
	gitEntry, _ := root.Lstat(".git")
	prefix := ""
	var info os.FileInfo
	for _, part := range strings.Split(path, "/") {
		if strings.EqualFold(strings.TrimRight(part, " ."), ".git") {
			return FilePreview{}, nil, errors.New("Git metadata cannot be previewed")
		}
		prefix = filepath.Join(prefix, part)
		info, err = root.Lstat(prefix)
		if errors.Is(err, os.ErrNotExist) {
			return FilePreview{Kind: "missing"}, nil, nil
		}
		if err != nil {
			return FilePreview{}, nil, err
		}
		if gitEntry != nil && os.SameFile(info, gitEntry) {
			return FilePreview{}, nil, errors.New("Git metadata cannot be previewed")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return FilePreview{Kind: "special"}, nil, nil
		}
	}
	if !info.Mode().IsRegular() {
		return FilePreview{Kind: "special"}, nil, nil
	}
	if info.Size() > previewBytes {
		return FilePreview{Kind: "large"}, nil, nil
	}
	file, err := root.Open(local)
	if errors.Is(err, os.ErrNotExist) {
		return FilePreview{Kind: "missing"}, nil, nil
	}
	if err != nil {
		return FilePreview{}, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return FilePreview{}, nil, err
	}
	if !os.SameFile(info, opened) {
		return FilePreview{}, nil, errors.New("the working file changed while opening it")
	}
	data, err := io.ReadAll(io.LimitReader(file, previewBytes+1))
	if err != nil {
		return FilePreview{}, nil, err
	}
	if len(data) > previewBytes || bytes.Count(data, []byte{'\n'}) > previewLines || bytes.Count(data, []byte{'\r'}) > previewLines {
		return FilePreview{Kind: "large"}, nil, nil
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return FilePreview{Kind: "binary"}, nil, nil
	}
	return FilePreview{Kind: "text", Content: string(data)}, opened, nil
}
