package git

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

type SyncState struct {
	HasHead        bool     `json:"hasHead"`
	Remotes        []string `json:"remotes"`
	Branch         string   `json:"branch"`
	UpstreamRemote string   `json:"upstreamRemote"`
	UpstreamBranch string   `json:"upstreamBranch"`
	TrackingRef    string   `json:"-"`
}

func ReadSyncState(ctx context.Context, dir string) (SyncState, error) {
	data, err := run(ctx, dir, nil, 1<<20, "remote")
	if err != nil {
		return SyncState{}, err
	}
	state := SyncState{Remotes: strings.Fields(string(data))}
	ref, err := headReference(ctx, dir)
	if err != nil || !strings.HasPrefix(ref, "refs/heads/") {
		return state, err
	}
	state.Branch = strings.TrimPrefix(ref, "refs/heads/")
	data, err = run(ctx, dir, nil, 1<<20, "for-each-ref",
		"--format=%(refname)%00%(upstream:remotename)%00%(upstream:remoteref)%00%(upstream)", "--", ref)
	if err != nil {
		return state, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) == 4 && fields[0] == ref {
			state.HasHead = true
			state.UpstreamRemote = fields[1]
			if strings.HasPrefix(fields[2], "refs/heads/") {
				state.UpstreamBranch = strings.TrimPrefix(fields[2], "refs/heads/")
			}
			state.TrackingRef = fields[3]
			break
		}
	}
	return state, nil
}

func FetchRemote(ctx context.Context, dir, remote string) error {
	data, err := run(ctx, dir, nil, 1<<20, "remote")
	if err != nil {
		return err
	}
	if !slices.Contains(strings.Fields(string(data)), remote) {
		return errors.New("select a configured Git remote")
	}
	return runRemote(ctx, dir, "fetch", "--no-recurse-submodules", "--", remote)
}

func PullUpstream(ctx context.Context, dir, branch, remote, target string) error {
	state, err := ReadSyncState(ctx, dir)
	if err != nil {
		return err
	}
	if branch == "" || state.Branch != branch {
		return errors.New("the current branch changed; review the synchronization target")
	}
	if !slices.Contains(state.Remotes, remote) || remote != state.UpstreamRemote ||
		target == "" || target != state.UpstreamBranch || !strings.HasPrefix(state.TrackingRef, "refs/remotes/") {
		return errors.New("the current branch needs a configured remote upstream")
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	head, err := headID(ctx, dir)
	if err != nil {
		return err
	}
	if err := requireCleanWorktree(ctx, dir); err != nil {
		return err
	}
	// Use a remote-tracking ref, since other Git clients can replace FETCH_HEAD.
	refspec := "+refs/heads/" + target + ":" + state.TrackingRef
	if err := runRemote(ctx, dir, "fetch", "--no-write-fetch-head", "--no-tags", "--no-prune", "--refmap=",
		"--no-recurse-submodules", "--", remote, refspec); err != nil {
		return err
	}
	current, err := ReadSyncState(ctx, dir)
	if err != nil {
		return err
	}
	currentHead, err := headID(ctx, dir)
	if err != nil {
		return err
	}
	if current.Branch != branch || currentHead != head || current.TrackingRef != state.TrackingRef ||
		current.UpstreamRemote != remote || current.UpstreamBranch != target {
		return errors.New("HEAD or its upstream changed during fetch; review the repository before pulling again")
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	if err := requireCleanWorktree(ctx, dir); err != nil {
		return err
	}
	_, err = run(ctx, dir, nil, 1<<20, "-c", "submodule.recurse=false", "merge",
		"--ff-only", "--no-squash", "--commit", "--no-autostash", "--no-overwrite-ignore", "--", state.TrackingRef)
	return remoteFailure(err)
}

func PushBranch(ctx context.Context, dir, branch, remote, target string) error {
	return pushBranch(ctx, dir, branch, remote, target, "")
}

func pushBranch(ctx context.Context, dir, branch, remote, target, expectedAddress string) error {
	state, err := ReadSyncState(ctx, dir)
	if err != nil {
		return err
	}
	if branch == "" || state.Branch != branch {
		return errors.New("the current branch changed; review the synchronization target")
	}
	if !slices.Contains(state.Remotes, remote) {
		return errors.New("select a configured Git remote")
	}
	if err := checkOperationState(ctx, dir); err != nil {
		return err
	}
	if _, err := headID(ctx, dir); err != nil {
		return err
	}
	args := []string{"push", "--no-force", "--no-mirror", "--no-prune", "--no-tags",
		"--no-follow-tags", "--recurse-submodules=no"}
	if state.UpstreamRemote == "" && target == branch {
		args = append(args, "--set-upstream")
	} else if state.UpstreamRemote != remote || state.UpstreamBranch != target || target == "" {
		return errors.New("the push target changed; review the branch upstream")
	}
	if expectedAddress != "" {
		effective, err := run(ctx, dir, nil, 32768, "remote", "get-url", "--push", "--all", "--", remote)
		if err != nil || strings.TrimSuffix(strings.TrimSuffix(string(effective), "\n"), "\r") != expectedAddress {
			return errors.New("the effective push destination changed; review the push URL in Manage remotes")
		}
	}
	return runRemote(ctx, dir, append(args, "--", remote, "refs/heads/"+branch+":refs/heads/"+target)...)
}

func runRemote(ctx context.Context, dir string, args ...string) error {
	_, err := runWithTimeout(ctx, 3*time.Minute, dir, nil, 2<<20, args...)
	return remoteFailure(err)
}

var remoteURL = regexp.MustCompile(`(?i)(https?|ssh|ftps?)://[^\s'"]+`)

func remoteFailure(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("Git synchronization timed out; check the connection and remote, then refresh before retrying")
	}
	detail := remoteURL.ReplaceAllString(err.Error(), "<remote URL>")
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "authentication failed"), strings.Contains(lower, "terminal prompts disabled"),
		strings.Contains(lower, "permission denied"), strings.Contains(lower, "could not read username"):
		return fmt.Errorf("remote authentication failed; check your Git credential helper or SSH agent, or authenticate in a terminal: %s", detail)
	case strings.Contains(lower, "non-fast-forward"), strings.Contains(lower, "not possible to fast-forward"),
		strings.Contains(lower, "[rejected]"), strings.Contains(lower, "updates were rejected"):
		return fmt.Errorf("synchronization rejected; review the branch history before retrying: %s", detail)
	case strings.Contains(lower, "could not resolve host"), strings.Contains(lower, "failed to connect"),
		strings.Contains(lower, "connection timed out"):
		return fmt.Errorf("remote connection failed: %s", detail)
	default:
		return errors.New(detail)
	}
}
