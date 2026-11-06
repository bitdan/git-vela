package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type HunkPatch struct {
	Revision string   `json:"revision"`
	Hunks    []string `json:"hunks"`
	Reason   string   `json:"reason"`
}

func WorkingHunks(ctx context.Context, dir, path string, staged bool) (HunkPatch, error) {
	patch, _, err := readHunks(ctx, dir, path, staged)
	return patch, err
}

func SetHunkStaged(ctx context.Context, dir, path string, staged bool, revision string, index int) error {
	return SetHunkLinesStaged(ctx, dir, path, staged, revision, index, nil)
}

func SetHunkLinesStaged(ctx context.Context, dir, path string, staged bool, revision string, index int, selected []int) error {
	patch, header, err := readHunks(ctx, dir, path, staged)
	if err != nil {
		return err
	}
	if revision == "" || patch.Revision != revision || index < 0 || index >= len(patch.Hunks) {
		return errors.New("the file diff changed; refresh the hunks and try again")
	}
	args := []string{"apply", "--cached", "--whitespace=nowarn"}
	hunk := patch.Hunks[index]
	if selected != nil {
		hunk, err = selectHunkLines(hunk, selected, staged)
		if err != nil {
			return err
		}
		// The partial patch produces a different blob. Keep only its path headers.
		var paths strings.Builder
		for _, line := range strings.SplitAfter(header, "\n") {
			if !strings.HasPrefix(line, "index ") {
				paths.WriteString(line)
			}
		}
		header = paths.String()
	} else if staged {
		args = append(args, "--reverse")
	}
	_, err = run(ctx, dir, []byte(header+hunk), 1<<20, append(args, "--", "-")...)
	return err
}

func readHunks(ctx context.Context, dir, path string, staged bool) (HunkPatch, string, error) {
	file, err := findWorkingFile(ctx, dir, path)
	if err != nil {
		return HunkPatch{}, "", err
	}
	patch := HunkPatch{Hunks: []string{}}
	side := file.Status[1]
	if staged {
		side = file.Status[0]
	}
	if side != 'M' || (!file.CanStage && !file.CanUnstage) {
		patch.Reason = "Hunk staging is available for modified, tracked text files."
		return patch, "", nil
	}

	args := []string{"diff", "--patch", "--no-ext-diff", "--no-textconv", "--no-color", "--no-relative", "--no-renames",
		"--ignore-submodules=all", "--src-prefix=a/", "--dst-prefix=b/", "--full-index", "--unified=3",
		"--output-indicator-new=+", "--output-indicator-old=-", "--output-indicator-context= "}
	if staged {
		args = append(args, "--cached")
	}
	data, err := run(ctx, dir, nil, previewBytes, append(append(args, "--"), ":(top,literal)"+path)...)
	if errors.Is(err, errLimit) || bytes.Count(data, []byte{'\n'}) > previewLines {
		patch.Reason = "The diff exceeds the hunk preview limit."
		return patch, "", nil
	}
	if err != nil {
		return patch, "", err
	}
	text := string(data)
	first := strings.Index(text, "\n@@ ")
	if first < 0 || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 || strings.Contains(text, "\ndiff --git ") {
		patch.Reason = "No compatible text hunks are available."
		return patch, "", nil
	}
	header := text[:first+1]
	if strings.Contains(header, "\nold mode ") || strings.Contains(header, "\nnew mode ") {
		patch.Reason = "Stage file mode changes with the whole file."
		return patch, "", nil
	}
	for start := first + 1; start < len(text); {
		next := strings.Index(text[start:], "\n@@ ")
		end := len(text)
		if next >= 0 {
			end = start + next + 1
		}
		patch.Hunks = append(patch.Hunks, text[start:end])
		start = end
	}
	patch.Revision = textRevision(text)
	return patch, header, nil
}

var hunkRange = regexp.MustCompile(`^@@ -([0-9]+)(?:,[0-9]+)? \+([0-9]+)(?:,[0-9]+)? @@`)

func selectHunkLines(hunk string, selected []int, reverse bool) (string, error) {
	lines := strings.Split(strings.TrimSuffix(hunk, "\n"), "\n")
	ranges := hunkRange.FindStringSubmatch(lines[0])
	if len(ranges) != 3 {
		return "", errors.New("invalid hunk range")
	}
	from := ranges[1]
	if reverse {
		from = ranges[2]
	}
	fromStart, err := strconv.Atoi(from)
	if err != nil {
		return "", err
	}

	chosen := make(map[int]bool, len(selected))
	for _, index := range selected {
		if index <= 0 || index >= len(lines) || len(lines[index]) == 0 ||
			(lines[index][0] != '+' && lines[index][0] != '-') {
			return "", errors.New("select added or removed lines from this hunk")
		}
		chosen[index] = true
	}
	if len(chosen) == 0 {
		return "", errors.New("select at least one changed line")
	}

	var body strings.Builder
	fromCount, toCount := 0, 0
	keepMarker := false
	for index, line := range lines[1:] {
		if line == "" {
			return "", errors.New("invalid empty patch record")
		}
		prefix := line[0]
		if prefix == '\\' {
			if keepMarker {
				body.WriteString(line + "\n")
			}
			continue
		}
		if reverse {
			switch prefix {
			case '+':
				prefix = '-'
			case '-':
				prefix = '+'
			}
		}
		keepMarker = true
		switch prefix {
		case '-':
			if !chosen[index+1] {
				prefix = ' '
			}
		case '+':
			if !chosen[index+1] {
				keepMarker = false
				continue
			}
		case ' ':
		default:
			return "", errors.New("invalid hunk line")
		}
		if prefix != '+' {
			fromCount++
		}
		if prefix != '-' {
			toCount++
		}
		body.WriteByte(prefix)
		body.WriteString(line[1:] + "\n")
	}

	toStart := fromStart
	switch {
	case fromCount == 0:
		toStart++
	case toCount == 0:
		toStart--
	}
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@\n%s", fromStart, fromCount, toStart, toCount, body.String()), nil
}
