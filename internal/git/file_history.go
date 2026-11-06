package git

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
)

type FileHistoryPage struct {
	Commits []Commit `json:"commits"`
	More    bool     `json:"more"`
}

type BlameLine struct {
	Number      int    `json:"number"`
	Hash        string `json:"hash"`
	Author      string `json:"author"`
	Subject     string `json:"subject"`
	Text        string `json:"text"`
	Uncommitted bool   `json:"uncommitted"`
}

func FileHistory(ctx context.Context, dir, path string) (FileHistoryPage, error) {
	if err := validateHistoryPath(path); err != nil {
		return FileHistoryPage{}, err
	}
	if _, err := headID(ctx, dir); err != nil {
		return FileHistoryPage{}, err
	}
	data, err := run(ctx, dir, nil, 8<<20, "--literal-pathspecs", "log", "--follow",
		"--no-show-signature", "--no-notes", "--encoding=UTF-8", "-z", "--format=%H%x00%P%x00%an%x00%at%x00%s",
		"-n", strconv.Itoa(pageSize+1), "HEAD", "--", path)
	if err != nil {
		return FileHistoryPage{}, err
	}
	commits, more, err := parseCommitPage(data, nil)
	return FileHistoryPage{Commits: commits, More: more}, err
}

func FileBlame(ctx context.Context, dir, path string) ([]BlameLine, error) {
	if err := validateHistoryPath(path); err != nil {
		return nil, err
	}
	preview, err := WorkingPreview(dir, path)
	if err != nil {
		return nil, err
	}
	if preview.Kind != "text" {
		return nil, errors.New("blame requires a regular UTF-8 working file within the preview limit")
	}
	data, err := run(ctx, dir, nil, 16<<20, "--literal-pathspecs", "blame", "--line-porcelain", "--encoding=UTF-8", "--", path)
	if err != nil {
		return nil, err
	}

	lines := []BlameLine{}
	var line BlameLine
	for _, field := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		switch {
		case strings.HasPrefix(field, "\t"):
			if line.Number != len(lines)+1 {
				return nil, errors.New("invalid Git blame line number")
			}
			line.Text = strings.TrimSuffix(field[1:], "\r")
			lines = append(lines, line)
			line = BlameLine{}
		case strings.HasPrefix(field, "author "):
			line.Author = strings.TrimPrefix(field, "author ")
		case strings.HasPrefix(field, "summary "):
			line.Subject = strings.TrimPrefix(field, "summary ")
		case line.Number == 0:
			fields := strings.Fields(field)
			if len(fields) >= 3 && objectID(fields[0]) {
				line.Hash = fields[0]
				line.Number, _ = strconv.Atoi(fields[2])
				line.Uncommitted = strings.Trim(line.Hash, "0") == ""
			}
		}
	}
	return lines, nil
}

func validateHistoryPath(path string) error {
	if local, err := filepath.Localize(path); err != nil || local == "." {
		return errors.New("select a repository file")
	}
	for _, part := range strings.Split(path, "/") {
		if strings.EqualFold(strings.TrimRight(part, " ."), ".git") {
			return errors.New("select a working file outside Git metadata")
		}
	}
	return nil
}
