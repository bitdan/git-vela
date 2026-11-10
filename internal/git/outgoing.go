package git

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// outgoingLimit matches IDEA GitOutgoingCommitsProvider's --max-count for a
// branch that does not exist on the remote yet.
const outgoingLimit = 1000

// Outgoing lists the commits a push would publish, for the Push dialog.
type Outgoing struct {
	// Tip is the local branch tip the list was read from; PushOptions.ExpectedTip
	// passes it back so a branch that moved after the review is not pushed.
	Tip string `json:"tip"`
	// NewBranch is set when remote/target has no remote-tracking ref, so the
	// list holds commits not on any branch of that remote.
	NewBranch bool     `json:"newBranch"`
	Commits   []Commit `json:"commits"`
	More      bool     `json:"more"`
}

// OutgoingCommits mirrors IDEA GitOutgoingCommitsProvider: target..source for
// an existing remote branch, otherwise source --not --remotes=<remote>.
// Remote-tracking refs are as fresh as the last fetch, as in IDEA.
func OutgoingCommits(ctx context.Context, dir, branch, remote, target string) (Outgoing, error) {
	var result Outgoing
	if err := checkBranchName(ctx, dir, branch); err != nil {
		return result, err
	}
	if target == "" || strings.HasPrefix(target, "-") || checkBranchName(ctx, dir, target) != nil {
		return result, errors.New("enter a valid remote branch name")
	}
	if err := requireRemote(ctx, dir, remote); err != nil {
		return result, err
	}
	tip, err := revision(ctx, dir, "refs/heads/"+branch)
	if err != nil {
		return result, errors.New("the branch has no commits to push")
	}
	result.Tip = tip
	args := []string{"log", "--topo-order", "--no-show-signature", "--no-notes", "--encoding=UTF-8", "-z",
		"--format=%H%x00%P%x00%an%x00%at%x00%s", "-n", strconv.Itoa(outgoingLimit + 1)}
	if base, err := revision(ctx, dir, "refs/remotes/"+remote+"/"+target); err == nil {
		args = append(args, base+".."+tip)
	} else {
		result.NewBranch = true
		args = append(args, tip, "--not", "--remotes="+remote)
	}
	data, err := run(ctx, dir, nil, 8<<20, append(args, "--")...)
	if err != nil {
		return result, err
	}
	result.Commits, result.More, err = parseOutgoing(data)
	return result, err
}

func parseOutgoing(data []byte) ([]Commit, bool, error) {
	commits := []Commit{}
	if len(data) == 0 {
		return commits, false, nil
	}
	fields := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	if len(fields)%5 != 0 {
		return nil, false, errors.New("invalid Git log record")
	}
	for i := 0; i < len(fields); i += 5 {
		if len(commits) == outgoingLimit {
			return commits, true, nil
		}
		ts, err := strconv.ParseInt(fields[i+3], 10, 64)
		if err != nil {
			return nil, false, err
		}
		commits = append(commits, Commit{
			Hash: fields[i], Parents: strings.Fields(fields[i+1]), Author: fields[i+2], Time: ts, Subject: fields[i+4],
		})
	}
	return commits, false, nil
}
