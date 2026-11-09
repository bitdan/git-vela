package git

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
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

// Fetch tag modes mirror IDEA's "Fetch tags" setting. The empty mode keeps the
// Git default: tags that point into the fetched history are updated, others are
// left alone.
const (
	FetchTagsReachable = ""
	FetchTagsAll       = "all"
	FetchTagsNone      = "none"
)

type FetchOptions struct {
	Prune bool   `json:"prune"`
	Tags  string `json:"tags"`
}

func FetchRemote(ctx context.Context, dir, remote string, options FetchOptions) error {
	data, err := run(ctx, dir, nil, 1<<20, "remote")
	if err != nil {
		return err
	}
	if !slices.Contains(strings.Fields(string(data)), remote) {
		return errors.New("select a configured Git remote")
	}
	args := []string{"fetch", "--no-recurse-submodules"}
	if options.Prune {
		args = append(args, "--prune")
	}
	switch options.Tags {
	case FetchTagsReachable:
		// Keep Git's default: update tags that point into fetched history and
		// honor remote.<name>.tagOpt.
	case FetchTagsAll:
		args = append(args, "--tags")
	case FetchTagsNone:
		args = append(args, "--no-tags")
	default:
		return errors.New("select a supported tag mode: all, reachable, or none")
	}
	return runRemote(ctx, dir, append(args, "--", remote)...)
}

// Pull modes, aligned with IDEA GitPullSettings strategies.
const (
	PullFastForward = "ff"
	PullMerge       = "merge"
	PullRebase      = "rebase"
)

type PullOptions struct {
	Mode      string `json:"mode"`
	Autostash bool   `json:"autostash"`
}

type PushOptions struct {
	PushTags       bool `json:"pushTags"`
	SetUpstream    bool `json:"setUpstream"`
	ForceWithLease bool `json:"forceWithLease"`
}

func PullUpstream(ctx context.Context, dir, branch, remote, target string, options PullOptions) error {
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
	if !options.Autostash {
		if err := requireCleanWorktree(ctx, dir); err != nil {
			return err
		}
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
	if !options.Autostash {
		if err := requireCleanWorktree(ctx, dir); err != nil {
			return err
		}
	}
	switch options.Mode {
	case "", PullFastForward:
		_, err = run(ctx, dir, nil, 1<<20, "-c", "submodule.recurse=false", "merge",
			"--ff-only", "--no-squash", "--commit", "--no-autostash", "--no-overwrite-ignore", "--", state.TrackingRef)
	case PullMerge:
		args := []string{"-c", "submodule.recurse=false", "merge", "--no-squash", "--commit", "--no-overwrite-ignore"}
		if options.Autostash {
			args = append(args, "--autostash")
		} else {
			args = append(args, "--no-autostash")
		}
		args = append(args, "--", state.TrackingRef)
		_, err = run(ctx, dir, nil, 1<<20, args...)
	case PullRebase:
		args := []string{"-c", "submodule.recurse=false", "rebase"}
		if options.Autostash {
			args = append(args, "--autostash")
		}
		args = append(args, "--", state.TrackingRef)
		_, err = run(ctx, dir, nil, 1<<20, args...)
	default:
		return errors.New("select a supported pull mode: fast-forward, merge, or rebase")
	}
	return remoteFailure(err)
}

func PushBranch(ctx context.Context, dir, branch, remote, target string, options PushOptions) error {
	return pushBranch(ctx, dir, branch, remote, target, "", options)
}

func pushBranch(ctx context.Context, dir, branch, remote, target, expectedAddress string, options PushOptions) error {
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
	args := []string{"push", "--no-mirror", "--no-prune", "--recurse-submodules=no"}
	if options.PushTags {
		args = append(args, "--follow-tags")
	} else {
		args = append(args, "--no-tags", "--no-follow-tags")
	}
	if options.SetUpstream || (state.UpstreamRemote == "" && target == branch) {
		args = append(args, "--set-upstream")
	} else if state.UpstreamRemote != remote || state.UpstreamBranch != target || target == "" {
		return errors.New("the push target changed; review the branch upstream")
	}
	if options.ForceWithLease {
		lease, err := run(ctx, dir, nil, 4096, "rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+target)
		if err != nil || !objectID(strings.TrimSuffix(strings.TrimSuffix(string(lease), "\n"), "\r")) {
			return errors.New("force-with-lease needs a local remote-tracking ref for the target; fetch before forcing")
		}
		leaseID := strings.TrimSuffix(strings.TrimSuffix(string(lease), "\n"), "\r")
		args = append(args, "--force-with-lease=refs/heads/"+target+":"+leaseID)
	} else {
		args = append(args, "--no-force")
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

type BranchSync struct {
	Branch   string `json:"branch"`
	Upstream string `json:"upstream"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	Gone     bool   `json:"gone"`
}

var syncTrack = regexp.MustCompile(`\bahead (\d+)|\bbehind (\d+)`)

// BranchSyncCounts reports local branch ahead/behind state relative to each
// configured upstream. Values come from remote-tracking refs, so they are as
// fresh as the last fetch (the same source IDEA uses for its branch markers).
func BranchSyncCounts(ctx context.Context, dir string) ([]BranchSync, error) {
	data, err := run(ctx, dir, nil, 1<<20, "for-each-ref",
		"--format=%(refname:short)%00%(upstream:short)%00%(upstream:track,nobracket)", "refs/heads")
	if err != nil {
		return nil, err
	}
	var result []BranchSync
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 3 || fields[0] == "" {
			continue
		}
		entry := BranchSync{Branch: fields[0], Upstream: fields[1]}
		if fields[2] == "gone" {
			entry.Gone = true
		}
		for _, match := range syncTrack.FindAllStringSubmatch(fields[2], -1) {
			if match[1] != "" {
				entry.Ahead = parseCount(match[1])
			}
			if match[2] != "" {
				entry.Behind = parseCount(match[2])
			}
		}
		result = append(result, entry)
	}
	return result, nil
}

func parseCount(value string) int {
	count, _ := strconv.Atoi(value)
	if count < 0 {
		return 0
	}
	return count
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
