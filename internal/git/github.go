package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

type PublishRequest struct {
	Account string `json:"account"`
	Name    string `json:"name"`
	Branch  string `json:"branch"`
	Private bool   `json:"private"`
}

type PublishResult struct {
	URL         string `json:"url"`
	RemoteAdded bool   `json:"remoteAdded"`
	Pushed      bool   `json:"pushed"`
	Failure     string `json:"failure"`
}

var githubAccount = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
var githubRepository = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

func GitHubLogin(ctx context.Context, dir string) (string, error) {
	client, err := githubCredentials(ctx, dir)
	if err != nil {
		return "", err
	}
	return client.login(ctx)
}

func PublishGitHub(ctx context.Context, dir string, req PublishRequest) (PublishResult, error) {
	return publishGitHub(ctx, dir, req, func(ctx context.Context, req PublishRequest) (string, error) {
		client, err := githubCredentials(ctx, dir)
		if err != nil {
			return "", err
		}
		return client.create(ctx, req)
	})
}

func publishGitHub(ctx context.Context, dir string, req PublishRequest, create func(context.Context, PublishRequest) (string, error)) (PublishResult, error) {
	if !githubAccount.MatchString(req.Account) || !githubRepository.MatchString(req.Name) ||
		req.Name == "." || req.Name == ".." || strings.HasSuffix(strings.ToLower(req.Name), ".git") {
		return PublishResult{}, errors.New("check the GitHub account and repository name")
	}
	state, err := ReadSyncState(ctx, dir)
	if err != nil {
		return PublishResult{}, err
	}
	if !state.HasHead || req.Branch == "" || req.Branch != state.Branch {
		return PublishResult{}, errors.New("create a commit on the displayed local branch before publishing")
	}
	if slices.Contains(state.Remotes, "origin") || state.UpstreamRemote != "" {
		return PublishResult{}, errors.New("this branch already has an upstream or the repository already has an origin; use Manage remotes or Sync")
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return PublishResult{}, err
	}
	head, err := headID(ctx, dir)
	if err != nil {
		return PublishResult{}, err
	}

	address, err := create(ctx, req)
	if err != nil {
		return PublishResult{}, err
	}
	result := PublishResult{URL: "https://github.com/" + req.Account + "/" + req.Name}
	if err := AddRemote(ctx, dir, "origin", address); err != nil {
		result.Failure = err.Error()
		return result, nil
	}
	result.RemoteAdded = true

	current, err := headID(ctx, dir)
	if err != nil || current != head {
		result.Failure = "HEAD changed during publication; review your branch before pushing"
		return result, nil
	}
	// Keep fetch and push on the GitHub repository the user approved.
	effective, err := run(ctx, dir, nil, 32768, "remote", "get-url", "--all", "origin")
	if err != nil || outputLine(effective) != address {
		result.Failure = "Git rewrote the fetch destination; review the effective URL in Manage remotes"
		return result, nil
	}
	if err := pushBranch(ctx, dir, req.Branch, "origin", req.Branch, address, PushOptions{}); err != nil {
		result.Failure = err.Error()
		return result, nil
	}
	result.Pushed = true
	return result, nil
}

type githubClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func githubCredentials(ctx context.Context, dir string) (githubClient, error) {
	output, err := runWithTimeout(ctx, 2*time.Minute, dir, []byte("protocol=https\nhost=github.com\n\n"), 32768, "credential", "fill")
	if err != nil {
		return githubClient{}, errors.New("GitHub credentials are unavailable; sign in using your Git credential helper, then try again")
	}
	for _, field := range strings.Split(string(output), "\n") {
		value, found := strings.CutPrefix(strings.TrimSuffix(field, "\r"), "password=")
		if found && value != "" {
			return githubClient{
				baseURL: "https://api.github.com", token: value,
				http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				}},
			}, nil
		}
	}
	return githubClient{}, errors.New("the Git credential helper returned no GitHub access token")
}

func (c githubClient) login(ctx context.Context) (string, error) {
	var account struct {
		Login string `json:"login"`
	}
	if err := c.request(ctx, http.MethodGet, "/user", nil, http.StatusOK, &account); err != nil {
		return "", err
	}
	if !githubAccount.MatchString(account.Login) {
		return "", errors.New("GitHub did not return a supported personal account")
	}
	return account.Login, nil
}

func (c githubClient) create(ctx context.Context, req PublishRequest) (string, error) {
	account, err := c.login(ctx)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(account, req.Account) {
		return "", errors.New("the signed-in GitHub account changed; check the account before publishing")
	}
	payload, err := json.Marshal(struct {
		Name    string `json:"name"`
		Private bool   `json:"private"`
	}{req.Name, req.Private})
	if err != nil {
		return "", err
	}
	var repo struct {
		FullName string `json:"full_name"`
		Private  bool   `json:"private"`
	}
	fullName := account + "/" + req.Name
	if err := c.request(ctx, http.MethodPost, "/user/repos", payload, http.StatusCreated, &repo); err != nil {
		return "", fmt.Errorf("GitHub creation failed for %s; check the repository before retrying: %w", fullName, err)
	}
	if !strings.EqualFold(repo.FullName, fullName) || repo.Private != req.Private {
		return "", fmt.Errorf("GitHub returned unexpected repository details; check %s before retrying", fullName)
	}
	return "https://github.com/" + fullName + ".git", nil
}

func (c githubClient) request(ctx context.Context, method, path string, payload []byte, expected int, output any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "GitVela")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach the GitHub API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != expected {
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("GitHub rejected the credentials or permissions (HTTP %d); check your credential helper", response.StatusCode)
		case http.StatusUnprocessableEntity:
			return errors.New("GitHub rejected the repository name or a repository with that name already exists")
		default:
			return fmt.Errorf("unexpected GitHub API response (HTTP %d)", response.StatusCode)
		}
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}
