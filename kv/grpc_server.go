package kv

import (
	"context"
	"errors"
	"strings"

	"github.com/Sjk4824/distributed_kv_store/api"
)

type Server struct {
	api.UnimplementedKVServer
	Store  *Store
	NodeID string
}

func NewServer(store *Store, nodeID string) *Server {
	return &Server{Store: store, NodeID: nodeID}
}

func (s *Server) Put(ctx context.Context, req *api.PutRequest) (*api.PutResponse, error) {
	key := strings.TrimSpace(req.GetKey())
	if key == "" {
		return nil, errors.New("key must be non empty")
	}
	if req.GetClientId() == "" {
		return nil, errors.New("client ID must be non empty")
	}

	_ = s.Store.ApplyPut(req.GetClientId(), req.GetRequestId(), key, req.GetValue())
	return &api.PutResponse{Ok: true, Leader: ""}, nil
}

func (s *Server) Get(ctx context.Context, req *api.GetRequest) (*api.GetResponse, error) {
	key := strings.TrimSpace(req.GetKey())
	if key == "" {
		return nil, errors.New("key must be non empty")
	}
	val, ok := s.Store.Get(key)
	if !ok {
		return &api.GetResponse{Found: false, Value: nil, Leader: ""}, nil
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

	_ = s.Store.ApplyDelete(req.GetClientId(), req.GetRequestId(), key)
	return &api.DeleteResponse{Ok: true, Leader: ""}, nil
}

func (s *Server) Health(ctx context.Context, req *api.HealthRequest) (*api.HealthResponse, error) {
	return &api.HealthResponse{
		NodeId:   s.NodeID,
		IsLeader: true,
		Term:     0,
	}, nil
}
