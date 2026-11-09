package git

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type ProjectEntry struct {
	Path     string `json:"path"`
	Folder   bool   `json:"folder"`
	Revision string `json:"revision"`
}

type ProjectMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func openProjectTree(dir string) (*os.Root, error) {
	if state := CheckRepositoryPath(dir); !state.Available {
		return nil, errors.New(state.Reason)
	}
	return os.OpenRoot(dir)
}

func projectInfo(root *os.Root, relative string) (string, fs.FileInfo, error) {
	if relative != "." {
		if err := validateHistoryPath(relative); err != nil {
			return "", nil, err
		}
	}
	local, err := filepath.Localize(relative)
	if err != nil {
		return "", nil, err
	}
	current := "."
	var info fs.FileInfo
	for _, part := range strings.Split(local, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err = root.Lstat(current)
		if err != nil {
			return "", nil, err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return "", nil, errors.New("select a regular file or directory inside the repository")
		}
	}
	return local, info, nil
}

func projectRevision(info fs.FileInfo) string {
	return fmt.Sprintf("%x:%x:%x", info.ModTime().UnixNano(), info.Size(), info.Mode())
}

func ProjectEntries(dir, relative string) ([]ProjectEntry, error) {
	root, err := openProjectTree(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	local, info, err := projectInfo(root, relative)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("select a project directory")
	}
	entries, err := fs.ReadDir(root.FS(), filepath.ToSlash(local))
	if err != nil {
		return nil, err
	}
	result := make([]ProjectEntry, 0, len(entries))
	for _, entry := range entries {
		if gitMetadataName(entry.Name()) {
			continue
		}
		item, err := entry.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if item.IsDir() || item.Mode().IsRegular() {
			result = append(result, ProjectEntry{Path: path.Join(relative, entry.Name()), Folder: item.IsDir(), Revision: projectRevision(item)})
		}
	}
	return result, nil
}

func gitMetadataName(name string) bool {
	return strings.EqualFold(strings.TrimRight(name, " ."), ".git")
}

func ProjectLocation(dir, relative string) (string, bool, error) {
	root, err := openProjectTree(dir)
	if err != nil {
		return "", false, err
	}
	defer root.Close()
	local, info, err := projectInfo(root, relative)
	if err != nil {
		return "", false, err
	}
	absolute, err := filepath.Abs(filepath.Join(dir, local))
	return absolute, info.IsDir(), err
}

func validProjectName(name string) bool {
	if name == "" || strings.TrimSpace(name) != name || strings.HasSuffix(name, ".") ||
		strings.ContainsAny(name, "/\\:<>|*?\"\x00\r\n") || gitMetadataName(name) {
		return false
	}
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	return len(base) != 4 || !(strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) || base[3] < '0' || base[3] > '9'
}

func projectDestination(root *os.Root, folder, name string) (string, error) {
	if !validProjectName(name) {
		return "", errors.New("enter one valid file or directory name")
	}
	dir, info, err := projectInfo(root, folder)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("select a destination directory")
	}
	dest := filepath.Join(dir, name)
	if _, err := root.Lstat(dest); err == nil {
		return "", errors.New("an item with this name already exists")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return dest, nil
}

// ChangeProjectEntry manages working files and folders without changing the index.
func ChangeProjectEntry(ctx context.Context, dir, action, source, revision, folder, name string) error {
	root, err := openProjectTree(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if action == "file" || action == "folder" {
		dest, err := projectDestination(root, folder, name)
		if err != nil {
			return err
		}
		if action == "folder" {
			return root.Mkdir(dest, 0755)
		}
		file, err := root.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		return file.Close()
	}
	if action != "copy" && action != "move" && action != "delete" {
		return errors.New("invalid project action")
	}
	if source == "." {
		return errors.New("the repository root cannot be changed")
	}
	local, info, err := projectInfo(root, source)
	if err != nil {
		return err
	}
	if revision != projectRevision(info) {
		return errors.New("the item changed on disk; refresh the project tree and try again")
	}
	// Recursive mutations must not remove nested Git metadata or follow links.
	if info.IsDir() {
		err := fs.WalkDir(root.FS(), source, func(item string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if gitMetadataName(entry.Name()) || (!entry.IsDir() && !entry.Type().IsRegular()) {
				return errors.New("the directory contains a nested repository or a special file")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if action == "delete" {
		return root.RemoveAll(local)
	}
	dest, err := projectDestination(root, folder, name)
	if err != nil {
		return err
	}
	if info.IsDir() && strings.HasPrefix(strings.ToLower(dest), strings.ToLower(local)+string(filepath.Separator)) {
		return errors.New("a directory cannot be placed inside itself")
	}
	if action == "move" {
		return root.Rename(local, dest)
	}
	return copyProjectEntry(ctx, root, local, dest, info)
}

func copyProjectEntry(ctx context.Context, root *os.Root, source, dest string, info fs.FileInfo) error {
	if !info.IsDir() {
		return copyProjectFile(root, source, dest, info.Mode().Perm())
	}
	// Publish the directory only after its contents were copied successfully.
	temp := filepath.Join(filepath.Dir(dest), ".gitvela-copy-"+rand.Text())
	if err := root.Mkdir(temp, 0755); err != nil {
		return err
	}
	defer root.RemoveAll(temp)
	err := fs.WalkDir(root.FS(), filepath.ToSlash(source), func(item string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if item == filepath.ToSlash(source) {
			return nil
		}
		local, info, err := projectInfo(root, item)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, local)
		if err != nil {
			return err
		}
		target := filepath.Join(temp, rel)
		if info.IsDir() {
			return root.Mkdir(target, info.Mode().Perm()|0700)
		}
		return copyProjectFile(root, local, target, info.Mode().Perm())
	})
	if err != nil {
		return err
	}
	if _, err := root.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
		return errors.New("the destination changed while copying")
	}
	return root.Rename(temp, dest)
}

func copyProjectFile(root *os.Root, source, dest string, mode fs.FileMode) error {
	in, err := root.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := root.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err := errors.Join(err, out.Close()); err != nil {
		_ = root.Remove(dest)
		return err
	}
	return nil
}

func SearchProject(ctx context.Context, dir, folder, query string) ([]ProjectMatch, error) {
	_, directory, err := ProjectLocation(dir, folder)
	if err != nil {
		return nil, err
	}
	if !directory || strings.TrimSpace(query) == "" || len(query) > 512 || strings.ContainsAny(query, "\x00\r\n") {
		return nil, errors.New("enter a search term and select a project directory")
	}
	args := []string{"grep", "-I", "-n", "-z", "--no-color", "--untracked", "-i", "-F", "-m", "20", "-e", query, "--"}
	if folder != "." {
		args = append(args, ":(top,literal)"+folder)
	}
	data, err := run(ctx, dir, nil, 2<<20, args...)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return []ProjectMatch{}, nil
	}
	if err != nil {
		return nil, err
	}
	matches := []ProjectMatch{}
	for len(data) > 0 && len(matches) < 200 {
		file, rest, ok := bytes.Cut(data, []byte{0})
		line, rest, okLine := bytes.Cut(rest, []byte{0})
		text, next, _ := bytes.Cut(rest, []byte{'\n'})
		number, err := strconv.Atoi(string(line))
		if !ok || !okLine || err != nil {
			return nil, errors.New("invalid Git search result")
		}
		matches = append(matches, ProjectMatch{Path: string(file), Line: number, Text: strings.TrimSpace(string(text))})
		data = next
	}
	return matches, nil
}
