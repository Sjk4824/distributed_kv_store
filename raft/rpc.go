package raft

import (
	"context"
	"log"

	"github.com/Sjk4824/distributed_kv_store/api"
)

func uptoDate(candidateIdx, candidateTerm, myIdx, myTerm uint64) bool {
	if candidateTerm != myTerm {
		return candidateTerm > myTerm
	}
	return candidateIdx >= myIdx
}

func (n *Node) RequestVote(ctx context.Context, req *api.RequestVoteRequest) (*api.RequestVoteResponse, error) {
	n.mu.Lock()
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

	myIdx, myTerm := n.lastLogIndexTermLocked()
	if !uptoDate(req.GetLastLogIndex(), req.GetLastLogTerm(), myIdx, myTerm) {
		term := n.term
		n.mu.Unlock()
		return &api.RequestVoteResponse{Term: term, VoteGranted: false}, nil
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

	// Check prevLogIndex/prevLogTerm match (log matching)
	prevIdx := req.GetPrevLogIndex()
	prevTerm := req.GetPrevLongTerm()

	// If we don't have prevLogIndex, reject
	if prevIdx > 0 && prevIdx-1 >= uint64(len(n.log)) {
		return &api.AppendEntriesResponse{Term: n.term, Success: false, MatchIndec: 0}, nil
	}

	// Check prevLogTerm matches
	if prevIdx > 0 && prevIdx-1 < uint64(len(n.log)) {
		if n.log[prevIdx-1].GetTerm() != prevTerm {
			return &api.AppendEntriesResponse{Term: n.term, Success: false, MatchIndec: 0}, nil
		}
	}

	// Append entries
	for _, entry := range req.GetEntries() {
		idx := entry.GetIndex()
		// If entry already exists, overwrite (handle conflicts)
		if idx-1 < uint64(len(n.log)) {
			n.log[idx-1] = entry
		} else {
			n.log = append(n.log, entry)
		}
	}

	// Update commitIndex based on leader's leaderCommit
	oldCommit := n.commitIndex
	if req.GetLeaderCommit() > n.commitIndex {
		// Commit up to min(leaderCommit, last log index)
		lastIdx := uint64(len(n.log))
		if req.GetLeaderCommit() < lastIdx {
			n.commitIndex = req.GetLeaderCommit()
		} else {
			n.commitIndex = lastIdx
		}
	}

	// Return the highest log index we now have
	matchIdx := uint64(len(n.log))

	select {
	case n.resetElectionCh <- struct{}{}:
	default:
	}

	log.Printf("[raft %s] AppendEntries accepted: entries=%d, commitIndex: %d->%d",
		n.id, len(req.GetEntries()), oldCommit, n.commitIndex)

	return &api.AppendEntriesResponse{Term: n.term, Success: true, MatchIndec: matchIdx}, nil
}
