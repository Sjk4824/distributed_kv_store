package raft

import (
	"context"
	"log"

	"github.com/Sjk4824/distributed_kv_store/api"
)

func (n *Node) RequestVote(ctx context.Context, req *api.RequestVoteRequest) (*api.RequestVoteResponse, error) {
	n.mu.Lock()

	// stale term -> reject
	if req.GetTerm() < n.term {
		term := n.term
		n.mu.Unlock()
		return &api.RequestVoteResponse{Term: term, VoteGranted: false}, nil
	}

	// newer term -> step down and clear prior vote
	if req.GetTerm() > n.term {
		n.term = req.GetTerm()
		n.role = Follower
		n.votedFor = ""
		n.leaderID = ""
	}

	// grant vote if haven't voted this term (or same candidate)
	if n.votedFor == "" || n.votedFor == req.GetCandidateId() {
		n.votedFor = req.GetCandidateId()
		term := n.term
		n.mu.Unlock()

		// reset election timer on granting vote
		select {
		case n.resetElectionCh <- struct{}{}:
		default:
		}

		return &api.RequestVoteResponse{Term: term, VoteGranted: true}, nil
	}

	term := n.term
	n.mu.Unlock()
	return &api.RequestVoteResponse{Term: term, VoteGranted: false}, nil
}

func (n *Node) AppendEntries(ctx context.Context, req *api.AppendEntriesRequest) (*api.AppendEntriesResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if req.GetTerm() < n.term {
		return &api.AppendEntriesResponse{Term: n.term, Success: false}, nil
	}

	if req.GetTerm() > n.term {
		n.term = req.GetTerm()
		n.votedFor = ""
	}

	n.role = Follower
	n.leaderID = req.GetLeaderId()

	select {
	case n.resetElectionCh <- struct{}{}:
	default:
	}
	log.Printf("[raft %s] heartbeat accepted from leader=%s term=%d", n.id, req.GetLeaderId(), req.GetTerm())
	return &api.AppendEntriesResponse{Term: n.term, Success: true}, nil
}
