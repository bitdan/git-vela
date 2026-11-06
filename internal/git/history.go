package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const pageSize = 200

type Ref struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
}

type Commit struct {
	Hash    string   `json:"hash"`
	Parents []string `json:"parents"`
	Author  string   `json:"author"`
	Time    int64    `json:"time"`
	Subject string   `json:"subject"`
	Refs    []string `json:"refs"`
	Graph   GraphRow `json:"graph"`
}

type Cursor struct {
	Offset   int    `json:"offset"`
	Snapshot string `json:"snapshot"`
	Lanes    []Lane `json:"lanes"`
}

type Page struct {
	Commits []Commit `json:"commits"`
	Refs    []Ref    `json:"refs"`
	More    bool     `json:"more"`
	Next    Cursor   `json:"next"`
}

func objectID(id string) bool {
	if len(id) != 40 && len(id) != 64 {
		return false
	}
	for _, ch := range id {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func History(ctx context.Context, dir string, cursor Cursor) (Page, error) {
	page := Page{Commits: []Commit{}, Refs: []Ref{}}
	if cursor.Offset < 0 || len(cursor.Lanes) > 4096 {
		return page, errors.New("invalid history cursor")
	}
	data, head, snapshot, err := readRefs(ctx, dir)
	if err != nil {
		return page, err
	}
	page.Next.Snapshot = snapshot
	if cursor.Snapshot != "" && cursor.Snapshot != snapshot {
		return page, errors.New("repository references changed; refresh the history")
	}
	labels := make(map[string][]string)
	var tips strings.Builder
	add := func(name, id string) {
		if !objectID(id) {
			return
		}
		if len(labels[id]) == 0 {
			tips.WriteString(id + "\n")
		}
		labels[id] = append(labels[id], name)
		page.Refs = append(page.Refs, Ref{Name: name, Hash: id})
	}
	add("HEAD", strings.TrimSpace(string(head)))
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 6 {
			continue
		}
		switch {
		case fields[1] == "commit":
			add(fields[0], fields[2])
		case fields[3] == "commit":
			add(fields[0], fields[4])
		}
	}
	if tips.Len() == 0 {
		return page, nil
	}
	data, err = run(ctx, dir, []byte(tips.String()), 8<<20, "log", "--topo-order", "--no-show-signature", "--no-notes", "--encoding=UTF-8", "-z",
		"--format=%H%x00%P%x00%an%x00%at%x00%s", "--skip="+strconv.Itoa(cursor.Offset), "-n", strconv.Itoa(pageSize+1), "--stdin", "--")
	if err != nil {
		return page, err
	}
	if len(data) == 0 {
		return page, nil
	}
	fields := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	if len(fields)%5 != 0 {
		return page, errors.New("invalid Git log record")
	}
	for i := 0; i < len(fields); i += 5 {
		if len(page.Commits) == pageSize {
			page.More = true
			break
		}
		ts, err := strconv.ParseInt(fields[i+3], 10, 64)
		if err != nil {
			return page, err
		}
		page.Commits = append(page.Commits, Commit{
			Hash: fields[i], Parents: strings.Fields(fields[i+1]), Author: fields[i+2],
			Time: ts, Subject: fields[i+4], Refs: labels[fields[i]],
		})
	}
	page.Next.Offset = cursor.Offset + len(page.Commits)
	page.Next.Lanes = layout(page.Commits, cursor.Lanes)
	return page, nil
}

func Snapshot(ctx context.Context, dir string) (string, error) {
	_, _, id, err := readRefs(ctx, dir)
	return id, err
}

func readRefs(ctx context.Context, dir string) ([]byte, []byte, string, error) {
	data, err := run(ctx, dir, nil, 4<<20, "for-each-ref", "--format=%(refname)%00%(objecttype)%00%(objectname)%00%(*objecttype)%00%(*objectname)%00%(HEAD)")
	if err != nil {
		return nil, nil, "", err
	}
	head, err := run(ctx, dir, nil, 4096, "rev-parse", "--revs-only", "HEAD")
	if err != nil {
		return nil, nil, "", err
	}
	digest := sha256.New()
	digest.Write(data)
	digest.Write(head)
	return data, head, fmt.Sprintf("%x", digest.Sum(nil)), nil
}
