package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type HistoryFilter struct {
	Message string `json:"message"`
	Author  string `json:"author"`
	Branch  string `json:"branch"`
	From    string `json:"from"`
	Through string `json:"through"`
	Hash    string `json:"hash"`
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
	args := []string{"--fixed-strings", "--regexp-ignore-case"}
	if f.Message != "" {
		args = append(args, "--grep="+f.Message)
	}
	if f.Author != "" {
		args = append(args, "--author="+f.Author)
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
	if f.Message != "" || f.Author != "" || f.Branch != "" || f.From != "" || f.Through != "" {
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

func (f HistoryFilter) snapshot(snapshot string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s %q", snapshot, f))))
}
