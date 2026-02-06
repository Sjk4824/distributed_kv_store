package kv

import (
	"context"
	"errors"
	"strings"

	"github.com/Sjk4824/distributed_kv_store/api"
	"github.com/Sjk4824/distributed_kv_store/raft"
)

type Server struct {
	api.UnimplementedKVServer
	DB     *DurableStore
	NodeID string
	Raft   *raft.Node
}

func NewServer(db *DurableStore, nodeID string, rn *raft.Node) *Server {
	return &Server{DB: db, NodeID: nodeID, Raft: rn}
}

func (s *Server) leaderHint() (isLeader bool, term uint64, leader string) {
	if s.Raft == nil {
		return true, 0, ""
	}
	role, t, l := s.Raft.RoleTermLeader()
	_ = role
	return s.Raft.IsLeader(), t, l
}

func (s *Server) Put(ctx context.Context, req *api.PutRequest) (*api.PutResponse, error) {
	key := strings.TrimSpace(req.GetKey())
	if key == "" {
		return nil, errors.New("key must be non empty")
	}
	if req.GetClientId() == "" {
		return nil, errors.New("client ID must be non empty")
	}

	isLeader, _, leader := s.leaderHint()
	if !isLeader {
		return &api.PutResponse{Ok: false, Leader: leader}, nil
	}

	// Create a command for raft logging
	cmd := &api.Command{
		ClientId:  req.GetClientId(),
		RequestId: req.GetRequestId(),
		Op: &api.Command_Put{
			Put: &api.Put{
				Key:   key,
				Value: req.GetValue(),
			},
		},
	}

	// Log it on the leader
	logIdx := s.Raft.AppendLog(cmd)
	if logIdx == 0 {
		// Lost leadership
		return &api.PutResponse{Ok: false, Leader: ""}, nil
	}

	// TODO: In production, wait for commitIndex to reach logIdx before responding
	// For now, return success immediately and let raft apply it asynchronously
	return &api.PutResponse{Ok: true, Leader: ""}, nil
}

func (s *Server) Get(ctx context.Context, req *api.GetRequest) (*api.GetResponse, error) {
	key := strings.TrimSpace(req.GetKey())
	if key == "" {
		return nil, errors.New("key must be non empty")
	}
	val, ok := s.DB.Get(key)
	if !ok {
		return &api.GetResponse{Found: false, Value: nil, Leader: ""}, nil
	}
	isLeader, _, leader := s.leaderHint()
	if !isLeader {
		return &api.GetResponse{Found: false, Value: nil, Leader: leader}, nil
	}
	return &api.GetResponse{Found: true, Value: val, Leader: ""}, nil
}

func (s *Server) Delete(ctx context.Context, req *api.DeleteRequest) (*api.DeleteResponse, error) {
	key := strings.TrimSpace(req.GetKey())
	if key == " " {
		return nil, errors.New("key must be non empty")
	}

	if req.GetClientId() == "" {
		return nil, errors.New("clinet ID must be non empty")
	}

	ifLeader, _, leader := s.leaderHint()
	if !ifLeader {
		return &api.DeleteResponse{Ok: false, Leader: leader}, nil
	}

	// Create a command for raft logging
	cmd := &api.Command{
		ClientId:  req.GetClientId(),
		RequestId: req.GetRequestId(),
		Op: &api.Command_Del{
			Del: &api.Delete{
				Key: key,
			},
		},
	}

	// Log it on the leader
	logIdx := s.Raft.AppendLog(cmd)
	if logIdx == 0 {
		// Lost leadership
		return &api.DeleteResponse{Ok: false, Leader: ""}, nil
	}

	// TODO: In production, wait for commitIndex to reach logIdx before responding
	return &api.DeleteResponse{Ok: true, Leader: ""}, nil
}

func (s *Server) Health(ctx context.Context, req *api.HealthRequest) (*api.HealthResponse, error) {
	isLeader, term, _ := s.leaderHint()
	return &api.HealthResponse{
		NodeId:   s.NodeID,
		IsLeader: isLeader,
		Term:     term,
	}, nil
}

// ApplyCommittedEntries applies raft log entries to the state machine
func (s *Server) ApplyCommittedEntries() error {
	if s.Raft == nil {
		return nil
	}

	return s.Raft.ApplyEntries(func(entry *api.LogEntry) error {
		cmd := entry.GetCmd()
		if cmd == nil {
			return nil
		}

		// Apply Put
		if put := cmd.GetPut(); put != nil {
			s.DB.ForcePut(put.GetKey(), put.GetValue())
			return nil
		}

		// Apply Delete
		if del := cmd.GetDel(); del != nil {
			s.DB.ForceDelete(del.GetKey())
			return nil
		}

		return nil
	})
}
