package git

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

type ConflictVersion struct {
	Stage   int    `json:"stage"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

func ConflictVersions(ctx context.Context, dir, path string) ([]ConflictVersion, error) {
	if err := validateHistoryPath(path); err != nil {
		return nil, err
	}
	data, err := run(ctx, dir, nil, 8192, "ls-files", "--unmerged", "-z", "--", ":(top,literal)"+path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("the selected file is no longer unmerged")
	}
	versions := []ConflictVersion{{Stage: 1, Kind: "missing"}, {Stage: 2, Kind: "missing"}, {Stage: 3, Kind: "missing"}}
	for _, record := range splitPaths(data) {
		metadata, name, found := strings.Cut(record, "\t")
		fields := strings.Fields(metadata)
		if !found || name != path || len(fields) != 3 || !objectID(fields[1]) {
			return nil, errors.New("invalid Git conflict stage")
		}
		stage, err := strconv.Atoi(fields[2])
		if err != nil || stage < 1 || stage > len(versions) {
			return nil, errors.New("invalid Git conflict stage number")
		}
		version := &versions[stage-1]
		if fields[0] == "160000" {
			version.Kind, version.Content = "submodule", fields[1]
			continue
		}
		content, err := run(ctx, dir, nil, previewBytes, "cat-file", "blob", fields[1])
		switch {
		case errors.Is(err, errLimit):
			version.Kind = "large"
		case err != nil:
			return nil, err
		case !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0:
			version.Kind = "binary"
		case bytes.Count(content, []byte{'\n'}) > previewLines:
			version.Kind = "large"
		default:
			version.Kind, version.Content = "text", string(content)
		}
	}
	return versions, nil
}
