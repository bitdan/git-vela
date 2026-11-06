package git

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInitializeProjectPreservesFilesAndStagesReview(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	write(t, dir, ".gitignore", ".env\n")
	write(t, dir, ".env", "local secret")
	write(t, dir, "read me.txt", "existing project")
	repo, err := Initialize(ctx, dir)
	if err != nil || repo.Head != "main" {
		t.Fatalf("initialized repository: %+v, %v", repo, err)
	}
	if err := StageAll(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if got := command(t, dir, "ls-files", "-z"); got != ".gitignore\x00read me.txt\x00" {
		t.Fatalf("unexpected staged paths: %q", got)
	}
	if _, err := Initialize(ctx, dir); err == nil {
		t.Fatal("initialized an existing repository")
	}
	if _, err := Initialize(ctx, t.TempDir()+"/missing"); err == nil {
		t.Fatal("accepted a nonexistent project folder")
	}
}

func TestManageRemotesAndPublishFirstBranch(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	if err := AddRemote(ctx, dir, "backup", "https://user:secret@example.invalid/project.git?token=private"); err != nil {
		t.Fatal(err)
	}
	remotes, err := ReadRemotes(ctx, dir)
	if err != nil || len(remotes) != 1 || remotes[0].FetchURL != "https://example.invalid/project.git" {
		t.Fatalf("remote details: %+v, %v", remotes, err)
	}
	if err := SetRemoteURL(ctx, dir, "backup", "https://example.invalid/other.git"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRemote(ctx, dir, "backup"); err != nil {
		t.Fatal(err)
	}
	if err := AddRemote(ctx, dir, "--mirror", "https://example.invalid/other.git"); err == nil {
		t.Fatal("accepted an option as the remote name")
	}

	bare := filepath.ToSlash(filepath.Join(t.TempDir(), "published.git"))
	command(t, dir, "init", "--bare", "-b", "main", bare)
	request := PublishRequest{Account: "test-owner", Name: "new-repo", Branch: "main", Private: true}
	create := func(_ context.Context, req PublishRequest) (string, error) {
		if req != request {
			return "", errors.New("unexpected publish request")
		}
		return bare, nil
	}
	if _, err := publishGitHub(ctx, dir, request, create); err == nil {
		t.Fatal("published before the first commit")
	}
	write(t, dir, "readme.txt", "project")
	commit(t, dir, "first commit")
	result, err := publishGitHub(ctx, dir, request, create)
	if err != nil || !result.Pushed || !result.RemoteAdded || result.Failure != "" {
		t.Fatalf("publish: %+v, %v", result, err)
	}
	if got := strings.TrimSpace(command(t, dir, "rev-parse", "--abbrev-ref", "@{upstream}")); got != "origin/main" {
		t.Fatalf("upstream = %q", got)
	}
	if _, err := publishGitHub(ctx, dir, request, create); err == nil {
		t.Fatal("published over an existing origin")
	}
}

func TestPublishKeepsNewRepositoryWhenPushFails(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "readme.txt", "project")
	commit(t, dir, "initial")
	request := PublishRequest{Account: "test-owner", Name: "new-repo", Branch: "main", Private: true}
	missing := filepath.ToSlash(filepath.Join(t.TempDir(), "missing.git"))
	result, err := publishGitHub(ctx, dir, request, func(context.Context, PublishRequest) (string, error) {
		return missing, nil
	})
	if err != nil || result.Pushed || !result.RemoteAdded || result.Failure == "" ||
		result.URL != "https://github.com/test-owner/new-repo" {
		t.Fatalf("partial publish: %+v, %v", result, err)
	}
}

func TestGitHubCreationUsesVerifiedAccountAndVisibility(t *testing.T) {
	ctx := context.Background()
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("request omitted the credential")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch req.Method + " " + req.URL.Path {
		case "GET /user":
			_ = json.NewEncoder(w).Encode(map[string]string{"login": "test-owner"})
		case "POST /user/repos":
			creates.Add(1)
			var payload struct {
				Name    string `json:"name"`
				Private bool   `json:"private"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.Name != "new-repo" || !payload.Private {
				t.Errorf("repository payload: %+v, %v", payload, err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"full_name": "test-owner/new-repo", "private": true})
		default:
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := githubClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	request := PublishRequest{Account: "different-owner", Name: "new-repo", Branch: "main", Private: true}
	if _, err := client.create(ctx, request); err == nil {
		t.Fatal("created a repository for a different account")
	}
	request.Account = "test-owner"
	address, err := client.create(ctx, request)
	if err != nil || address != "https://github.com/test-owner/new-repo.git" || creates.Load() != 1 {
		t.Fatalf("repository creation: address=%q creates=%d err=%v", address, creates.Load(), err)
	}
}

func TestPublishRejectsRewrittenPushDestination(t *testing.T) {
	ctx := context.Background()
	dir := repository(t)
	write(t, dir, "project.txt", "committed")
	commit(t, dir, "initial")
	wrong := filepath.ToSlash(filepath.Join(t.TempDir(), "other.git"))
	command(t, dir, "init", "--bare", "-b", "main", wrong)
	address := "https://github.com/test-owner/new-repo.git"
	command(t, dir, "config", "url."+wrong+".pushInsteadOf", address)
	req := PublishRequest{Account: "test-owner", Name: "new-repo", Branch: "main", Private: true}
	result, err := publishGitHub(ctx, dir, req, func(context.Context, PublishRequest) (string, error) {
		return address, nil
	})
	if err != nil || result.Pushed || !result.RemoteAdded || !strings.Contains(result.Failure, "push destination") {
		t.Fatalf("unsafe or unexpected publication result: %+v, %v", result, err)
	}
	if got := command(t, wrong, "for-each-ref"); got != "" {
		t.Fatalf("published into a rewritten destination: %q", got)
	}
}
