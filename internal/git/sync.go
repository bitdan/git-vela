package git

import (
	"context"
	"errors"
	"fmt"
	"path"
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
	args, err := fetchArgs(options)
	if err != nil {
		return err
	}
	return runRemote(ctx, dir, append(args, "--", remote)...)
}

// FetchAllRemotes updates every configured remote in one call. Git keeps the
// per-remote refspecs, so a failure of one remote is reported by the caller.
func FetchAllRemotes(ctx context.Context, dir string, options FetchOptions) error {
	data, err := run(ctx, dir, nil, 1<<20, "remote")
	if err != nil {
		return err
	}
	if len(strings.Fields(string(data))) == 0 {
		return errors.New("this repository has no remotes to fetch")
	}
	args, err := fetchArgs(options)
	if err != nil {
		return err
	}
	return runRemote(ctx, dir, append(args, "--all")...)
}

func fetchArgs(options FetchOptions) ([]string, error) {
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
		return nil, errors.New("select a supported tag mode: all, reachable, or none")
	}
	return args, nil
}

type PushOptions struct {
	PushTags       bool `json:"pushTags"`
	SetUpstream    bool `json:"setUpstream"`
	ForceWithLease bool `json:"forceWithLease"`
	// Protected branch patterns (exact names or shell wildcards such as
	// release/*) refuse a force push even after the confirmation dialog.
	Protected []string `json:"protected"`
}

// protectedBranch reports the pattern that matches a branch name, if any.
func protectedBranch(patterns []string, branch string) (string, bool) {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if pattern == branch {
			return pattern, true
		}
		if matched, err := path.Match(pattern, branch); err == nil && matched {
			return pattern, true
		}
	}
	return "", false
}

func PushBranch(ctx context.Context, dir, branch, remote, target string, options PushOptions) error {
	return pushBranch(ctx, dir, branch, remote, target, "", options)
}

func pushBranch(ctx context.Context, dir, branch, remote, target, expectedAddress string, options PushOptions) error {
	// Push acts on the branch the user selected (IDEA GitPushBranchAction), which
	// does not have to be checked out; its own upstream decides the default target.
	tracking, err := branchTracking(ctx, dir, branch)
	if err != nil {
		return err
	}
	remotes, err := remoteNames(ctx, dir)
	if err != nil {
		return err
	}
	if !slices.Contains(remotes, remote) {
		return errors.New("select a configured Git remote")
	}
	if headRef, err := headReference(ctx, dir); err != nil {
		return err
	} else if headRef == "refs/heads/"+branch {
		if err := checkOperationState(ctx, dir); err != nil {
			return err
		}
	}
	if _, err := revision(ctx, dir, "refs/heads/"+branch); err != nil {
		return errors.New("the branch has no commits to push")
	}
	if target == "" || strings.HasPrefix(target, "-") || checkBranchName(ctx, dir, target) != nil {
		return errors.New("enter a valid remote branch name")
	}
	state := struct{ UpstreamRemote, UpstreamBranch string }{tracking.Remote, tracking.RemoteBranch}
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
		if pattern, blocked := protectedBranch(options.Protected, target); blocked {
			return fmt.Errorf("force push to protected branch %s is blocked (pattern %s); remove the protection in Settings to override", target, pattern)
		}
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
