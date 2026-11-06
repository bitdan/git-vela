package git

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

type RemoteInfo struct {
	Name     string `json:"name"`
	FetchURL string `json:"fetchUrl"`
	PushURL  string `json:"pushUrl"`
}

func ReadRemotes(ctx context.Context, dir string) ([]RemoteInfo, error) {
	data, err := run(ctx, dir, nil, 1<<20, "remote", "-v")
	if err != nil {
		return nil, remoteFailure(err)
	}
	remotes := []RemoteInfo{}
	positions := make(map[string]int)
	for _, line := range strings.Split(string(data), "\n") {
		name, detail, _ := strings.Cut(line, "\t")
		if name == "" {
			continue
		}
		index, exists := positions[name]
		if !exists {
			index = len(remotes)
			positions[name] = index
			remotes = append(remotes, RemoteInfo{Name: name})
		}
		switch {
		case strings.HasSuffix(detail, " (fetch)"):
			appendRemoteURL(&remotes[index].FetchURL, strings.TrimSuffix(detail, " (fetch)"))
		case strings.HasSuffix(detail, " (push)"):
			appendRemoteURL(&remotes[index].PushURL, strings.TrimSuffix(detail, " (push)"))
		}
	}
	return remotes, nil
}

func appendRemoteURL(list *string, address string) {
	if strings.Contains(address, "://") {
		u, err := url.Parse(address)
		if err != nil {
			address = "[invalid URL]"
		} else {
			if u.User != nil {
				if _, hasPassword := u.User.Password(); hasPassword || u.Scheme != "ssh" {
					u.User = nil
				}
			}
			u.RawQuery, u.Fragment, u.ForceQuery = "", "", false
			address = u.String()
		}
	}
	if *list != "" {
		*list += "\n"
	}
	*list += address
}

func validRemoteName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && !strings.HasPrefix(name, "-") &&
		!strings.ContainsAny(name, "\x00\r\n")
}

func AddRemote(ctx context.Context, dir, name, address string) error {
	return changeRemote(ctx, dir, "add", name, address)
}

func SetRemoteURL(ctx context.Context, dir, name, address string) error {
	return changeRemote(ctx, dir, "set-url", name, address)
}

func RemoveRemote(ctx context.Context, dir, name string) error {
	if !validRemoteName(name) {
		return errors.New("select a valid remote name")
	}
	_, err := run(ctx, dir, nil, 1<<20, "remote", "remove", "--", name)
	return remoteFailure(err)
}

func changeRemote(ctx context.Context, dir, action, name, address string) error {
	if !validRemoteName(name) {
		return errors.New("enter a valid remote name")
	}
	if strings.TrimSpace(address) != address || address == "" ||
		len(address) > 4096 || strings.ContainsAny(address, "\x00\r\n\t") {
		return errors.New("enter a Git remote URL or local repository path")
	}
	_, err := run(ctx, dir, nil, 1<<20, "remote", action, "--", name, address)
	return remoteFailure(err)
}
