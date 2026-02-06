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

	if err := s.DB.Put(req.GetClientId(), req.GetRequestId(), key, req.GetValue()); err != nil {
		return nil, err
	}
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

	if err := s.DB.Delete(req.GetClientId(), req.GetRequestId(), key); err != nil {
		return nil, err
	}
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
