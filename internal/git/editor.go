package git

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

type TextFile struct {
	Content    string `json:"content"`
	Revision   string `json:"revision"`
	LineEnding string `json:"lineEnding"`
	BOM        bool   `json:"bom"`
}

func EditableFilePaths(ctx context.Context, dir string) ([]string, error) {
	data, err := run(ctx, dir, nil, 16<<20, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--")
	if err != nil {
		return nil, err
	}
	files := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	if len(data) == 0 {
		return []string{}, nil
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

func OpenTextFile(ctx context.Context, dir, path string) (TextFile, error) {
	root, err := openEditorRoot(ctx, dir)
	if err != nil {
		return TextFile{}, err
	}
	defer root.Close()
	file, _, err := readTextFile(root, path)
	return file, err
}

func SaveTextFile(ctx context.Context, dir, path, revision, content string) (string, error) {
	if len(content) > previewBytes || strings.Count(content, "\n") > previewLines {
		return "", errors.New("text exceeds the editor limit")
	}
	if !utf8.ValidString(content) || strings.ContainsAny(content, "\x00\r") {
		return "", errors.New("the editor expects UTF-8 text with normalized line endings")
	}

	root, err := openEditorRoot(ctx, dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	file, info, err := readTextFile(root, path)
	if err != nil {
		return "", err
	}
	if revision != file.Revision {
		return "", errors.New("the file changed on disk; reload it before saving")
	}
	if info.Mode().Perm()&0222 == 0 {
		return "", errors.New("the working file is read-only")
	}
	if content == file.Content {
		return revision, nil
	}

	if file.LineEnding == "CRLF" {
		content = strings.ReplaceAll(content, "\n", "\r\n")
	}
	if file.BOM {
		content = "\ufeff" + content
	}
	if len(content) > previewBytes {
		return "", errors.New("text exceeds the editor limit after restoring the original line endings")
	}
	local, err := filepath.Localize(path)
	if err != nil {
		return "", err
	}
	temp := filepath.Join(filepath.Dir(local), ".gitvela-edit-"+rand.Text())
	out, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer root.Remove(temp)
	writeErr := writeTextReplacement(out, content, info.Mode().Perm())
	closeErr := out.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	current, currentInfo, err := readTextFile(root, path)
	if err != nil {
		return "", err
	}
	if current.Revision != revision || !os.SameFile(info, currentInfo) || info.Mode() != currentInfo.Mode() {
		return "", errors.New("the file changed on disk; reload it before saving")
	}
	if err := root.Rename(temp, local); err != nil {
		return "", err
	}
	return textRevision(content), nil
}

func openEditorRoot(ctx context.Context, dir string) (*os.Root, error) {
	repo, err := Open(ctx, dir)
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(repo.Path)
}

func writeTextReplacement(file *os.File, content string, mode os.FileMode) error {
	if _, err := file.WriteString(content); err != nil {
		return err
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	return file.Sync()
}

func readTextFile(root *os.Root, path string) (TextFile, os.FileInfo, error) {
	preview, info, err := readWorkingPreview(root, path)
	if err != nil {
		return TextFile{}, nil, err
	}
	switch preview.Kind {
	case "missing":
		return TextFile{}, nil, errors.New("the working file is absent")
	case "large":
		return TextFile{}, nil, errors.New("the editor supports up to 512 KiB and 12,000 line breaks")
	case "binary":
		return TextFile{}, nil, errors.New("the editor supports UTF-8 text files")
	case "text":
	default:
		return TextFile{}, nil, errors.New("select a regular working file")
	}
	content, bom := strings.CutPrefix(preview.Content, "\ufeff")
	lineEnding := "LF"
	crlf := strings.Count(content, "\r\n")
	if crlf != 0 {
		lineEnding = "CRLF"
	}
	if strings.Count(content, "\r") != crlf || (crlf != 0 && strings.Count(content, "\n") != crlf) {
		return TextFile{}, nil, errors.New("the editor supports consistent LF or CRLF line endings")
	}
	return TextFile{
		Content:  strings.ReplaceAll(content, "\r\n", "\n"),
		Revision: textRevision(preview.Content), LineEnding: lineEnding, BOM: bom,
	}, info, nil
}

func textRevision(content string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}
