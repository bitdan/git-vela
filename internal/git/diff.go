package git

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

const previewBytes = 512 << 10
const previewLines = 12000

type Change struct {
	Index   int    `json:"index"`
	Path    string `json:"path"`
	OldPath string `json:"oldPath"`
	Status  string `json:"status"`
	oldMode string
	newMode string
	oldID   string
	newID   string
}

type Comparison struct {
	Kind   string `json:"kind"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func Files(ctx context.Context, dir, commit string, parent int) ([]Change, error) {
	if !objectID(commit) || parent < 0 {
		return nil, errors.New("invalid commit or parent")
	}
	args := diffTreeArgs()
	if parent == 0 {
		args = append(args, "--root", "--diff-merges=first-parent", commit+"^{commit}")
	} else {
		if parent >= 65536 {
			return nil, errors.New("invalid commit parent")
		}
		args = append(args, commit+"^"+strconv.Itoa(parent+1), commit+"^{commit}")
	}
	return readTreeChanges(ctx, dir, args)
}

func diffTreeArgs() []string {
	return []string{"diff-tree", "--no-commit-id", "-r", "--raw", "-z", "--no-abbrev", "-M", "-C", "--no-ext-diff", "--no-textconv"}
}

func CompareFiles(ctx context.Context, dir, before, after string) ([]Change, error) {
	if !objectID(before) || !objectID(after) {
		return nil, errors.New("select two valid commits")
	}
	return readTreeChanges(ctx, dir, append(diffTreeArgs(), before+"^{commit}", after+"^{commit}"))
}

func readTreeChanges(ctx context.Context, dir string, args []string) ([]Change, error) {
	data, err := run(ctx, dir, nil, 16<<20, append(args, "--")...)
	if err != nil {
		return nil, err
	}
	files := []Change{}
	if len(data) == 0 {
		return files, nil
	}
	fields := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	for i := 0; i < len(fields); {
		meta := strings.Fields(fields[i])
		if len(meta) != 5 || !strings.HasPrefix(meta[0], ":") || i+1 >= len(fields) {
			return nil, errors.New("invalid Git diff record")
		}
		file := Change{
			Index: len(files), Path: fields[i+1], Status: meta[4][:1],
			oldMode: meta[0][1:], newMode: meta[1], oldID: meta[2], newID: meta[3],
		}
		i += 2
		if file.Status == "R" || file.Status == "C" {
			if i >= len(fields) {
				return nil, errors.New("missing Git rename destination")
			}
			file.OldPath, file.Path = file.Path, fields[i]
			i++
		}
		files = append(files, file)
	}
	return files, nil
}

func Diff(ctx context.Context, dir, commit string, parent int, selected Change) (Comparison, error) {
	files, err := Files(ctx, dir, commit, parent)
	if err != nil {
		return Comparison{}, err
	}
	return compareFile(ctx, dir, files, selected.Index)
}

func CompareDiff(ctx context.Context, dir, before, after string, index int) (Comparison, error) {
	files, err := CompareFiles(ctx, dir, before, after)
	if err != nil {
		return Comparison{}, err
	}
	return compareFile(ctx, dir, files, index)
}

func compareFile(ctx context.Context, dir string, files []Change, index int) (Comparison, error) {
	if index < 0 || index >= len(files) {
		return Comparison{}, errors.New("file is unavailable in this commit comparison")
	}
	file := files[index]
	if file.oldMode == "160000" || file.newMode == "160000" {
		return Comparison{Kind: "submodule", Before: gitlink(file.oldMode, file.oldID), After: gitlink(file.newMode, file.newID)}, nil
	}
	sides, kind, err := blobs(ctx, dir, file)
	if err != nil || kind != "text" {
		return Comparison{Kind: kind}, err
	}
	if strings.HasPrefix(sides[0], "version https://git-lfs.github.com/spec/v1\n") || strings.HasPrefix(sides[1], "version https://git-lfs.github.com/spec/v1\n") {
		kind = "lfs"
	}
	return Comparison{Kind: kind, Before: sides[0], After: sides[1]}, nil
}

func gitlink(mode, id string) string {
	if mode == "160000" {
		return id
	}
	return ""
}

func blobs(ctx context.Context, dir string, file Change) ([2]string, string, error) {
	var sides [2]string
	var input strings.Builder
	ids := [2]string{file.oldID, file.newID}
	for i, mode := range [2]string{file.oldMode, file.newMode} {
		if mode == "000000" {
			ids[i] = ""
		} else {
			input.WriteString(ids[i] + "\n")
		}
	}
	data, err := run(ctx, dir, []byte(input.String()), 2*previewBytes+512, "cat-file", "--batch")
	if errors.Is(err, errLimit) {
		return sides, "large", nil
	}
	if err != nil {
		return sides, "", err
	}
	for i, id := range ids {
		if id == "" {
			continue
		}
		header, tail, found := bytes.Cut(data, []byte{'\n'})
		fields := strings.Fields(string(header))
		if !found || len(fields) != 3 || fields[0] != id || fields[1] != "blob" {
			return sides, "", errors.New("Git blob is unavailable")
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size >= len(tail) || tail[size] != '\n' {
			return sides, "", errors.New("invalid Git blob size")
		}
		if size > previewBytes {
			return sides, "large", nil
		}
		blob := tail[:size]
		if bytes.IndexByte(blob, 0) >= 0 || !utf8.Valid(blob) {
			return sides, "binary", nil
		}
		if bytes.Count(blob, []byte{'\n'}) > previewLines || bytes.Count(blob, []byte{'\r'}) > previewLines {
			return sides, "large", nil
		}
		sides[i] = string(blob)
		data = tail[size+1:]
	}
	return sides, "text", nil
}
