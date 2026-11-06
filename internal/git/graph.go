package git

import (
	"hash/fnv"
	"slices"
)

type Lane struct {
	Hash  string `json:"hash"`
	Color uint32 `json:"color"`
}

type Edge struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Color uint32 `json:"color"`
}

type GraphRow struct {
	Node     int    `json:"node"`
	Color    uint32 `json:"color"`
	Incoming bool   `json:"incoming"`
	Through  []Edge `json:"through"`
	Out      []Edge `json:"out"`
	Width    int    `json:"width"`
}

func laneColor(id string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(id))
	return hash.Sum32()
}

func lanePositions(lanes []Lane) map[string]int {
	positions := make(map[string]int, len(lanes))
	for index, lane := range lanes {
		positions[lane.Hash] = index
	}
	return positions
}

func layout(commits []Commit, lanes []Lane) []Lane {
	lanes = slices.Clone(lanes)
	positions := lanePositions(lanes)
	for i := range commits {
		commit := &commits[i]
		node, incoming := positions[commit.Hash]
		row := GraphRow{Color: laneColor(commit.Hash), Incoming: incoming}
		if !incoming {
			node = len(lanes)
		} else {
			row.Color = lanes[node].Color
		}
		row.Node = node
		next := make([]Lane, 0, len(lanes)+len(commit.Parents))
		next = append(next, lanes[:node]...)
		for index, parent := range commit.Parents {
			if _, exists := positions[parent]; exists {
				continue
			}
			color := laneColor(parent)
			if index == 0 {
				color = row.Color
			}
			next = append(next, Lane{Hash: parent, Color: color})
		}
		if incoming {
			next = append(next, lanes[node+1:]...)
		}
		positions = lanePositions(next)
		for from, lane := range lanes {
			if from != node {
				row.Through = append(row.Through, Edge{From: from, To: positions[lane.Hash], Color: lane.Color})
			}
		}
		for _, parent := range commit.Parents {
			to := positions[parent]
			row.Out = append(row.Out, Edge{From: node, To: to, Color: next[to].Color})
		}
		row.Width = max(node+1, len(lanes), len(next))
		commit.Graph = row
		lanes = next
	}
	return lanes
}
