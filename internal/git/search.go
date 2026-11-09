package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type HistoryFilter struct {
	Message     string `json:"message"`
	Author      string `json:"author"`
	Branch      string `json:"branch"`
	From        string `json:"from"`
	Through     string `json:"through"`
	Hash        string `json:"hash"`
	Path        string `json:"path,omitempty"`
	Regex       bool   `json:"regex,omitempty"`
	MatchCase   bool   `json:"matchCase,omitempty"`
	Sort        string `json:"sort,omitempty"`
	FirstParent bool   `json:"firstParent,omitempty"`
	NoMerges    bool   `json:"noMerges,omitempty"`
}

func SearchHistory(ctx context.Context, dir string, filter HistoryFilter, cursor Cursor) (Page, error) {
	return readHistory(ctx, dir, cursor, &filter)
}

func (f HistoryFilter) options() ([]string, error) {
	for _, value := range []string{f.Message, f.Author} {
		if len(value) > 1024 || strings.ContainsAny(value, "\x00\r\n") {
			return nil, errors.New("search text must fit on one line")
		}
	}
	args := []string{}
	if f.Regex {
		args = append(args, "--extended-regexp")
	} else {
		args = append(args, "--fixed-strings")
	}
	if !f.MatchCase {
		args = append(args, "--regexp-ignore-case")
	}
	if f.FirstParent {
		args = append(args, "--first-parent")
	}
	if f.NoMerges {
		args = append(args, "--no-merges")
	}
	if f.Message != "" {
		args = append(args, "--grep="+f.Message)
	}
	if f.Author != "" {
		author := f.Author
		if f.Regex {
			author = regexp.QuoteMeta(author)
		}
		args = append(args, "--author="+author)
	}
	var from time.Time
	if f.From != "" {
		var err error
		from, err = time.Parse(time.RFC3339, f.From)
		if err != nil {
			return nil, errors.New("invalid start date")
		}
		args = append(args, "--since-as-filter=@"+strconv.FormatInt(from.Unix(), 10))
	}
	if f.Through != "" {
		through, err := time.Parse(time.RFC3339, f.Through)
		if err != nil {
			return nil, errors.New("invalid end date")
		}
		if !from.IsZero() && through.Before(from) {
			return nil, errors.New("end date must be on or after the start date")
		}
		args = append(args, "--until=@"+strconv.FormatInt(through.Unix(), 10))
	}
	return args, nil
}

func (f HistoryFilter) hashTip(ctx context.Context, dir string) (string, error) {
	hash := strings.ToLower(f.Hash)
	if len(hash) < 7 || len(hash) > 64 || strings.Trim(hash, "0123456789abcdef") != "" {
		return "", errors.New("enter at least seven hexadecimal digits from a commit hash")
	}
	if f.Message != "" || f.Author != "" || f.Branch != "" || f.From != "" || f.Through != "" || f.Path != "" || f.FirstParent || f.NoMerges {
		return "", errors.New("search by hash separately from history filters")
	}
	data, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", "--quiet", "--end-of-options", hash+"^{commit}")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if !objectID(id) || !strings.HasPrefix(id, hash) {
		return "", nil
	}
	return id + "\n", nil
}

func (f HistoryFilter) pathspec() (string, error) {
	if _, err := filepath.Localize(f.Path); err != nil || strings.ContainsAny(f.Path, "\x00\r\n") {
		return "", errors.New("enter a repository-relative file or folder path")
	}
	return ":(top,literal)" + f.Path, nil
}

func (f HistoryFilter) graph() bool {
	return f.Message == "" && f.Author == "" && f.Path == "" && f.Hash == "" &&
		f.From == "" && f.Through == "" && !f.FirstParent && !f.NoMerges
}

func (f HistoryFilter) snapshot(snapshot string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s %#v", snapshot, f))))
}
