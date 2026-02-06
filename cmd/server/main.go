package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/Sjk4824/distributed_kv_store/api"
	"github.com/Sjk4824/distributed_kv_store/kv"
	"github.com/Sjk4824/distributed_kv_store/raft"
	"google.golang.org/grpc"
)

func main() {
	addr := flag.String("addr", ":50051", "The address to listen on")
	nodeID := flag.String("node", "node-1", "node id")
	walPath := flag.String("wal", "./data/wal.log", "path to wal file")
	snapEvery := flag.Duration("snap_every", 0, "snapshot interval (e.g. 5s, 30s). 0 disables snapshots")
	raftAddr := flag.String("raft_addr", ":60051", "raft listen address")
	peersCSV := flag.String("peers", "", "comma-seperated raft peer raft_addrs (e.g. 127.0.0.1:60052,127.0.0.1:60053)")
	flag.Parse()

	var peers []string
	if *peersCSV != "" {
		peers = strings.Split(*peersCSV, ",")
	}

	raftLis, err := net.Listen("tcp", *raftAddr)
	if err != nil {
		log.Fatalf("raft listen %s: %v", *raftAddr, err)
	}

	rn := raft.NewNode(*nodeID, *raftAddr, peers)
	rn.Start()
	defer rn.Stop()

	raftServer := grpc.NewServer()
	api.RegisterRaftServer(raftServer, rn)

	go func() {
		if err := raftServer.Serve(raftLis); err != nil {
			log.Fatalf("serve raft rpc: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}
	db, err := kv.OpenDurableStore(*walPath, *snapEvery)
	if err != nil {
		log.Fatalf("open durable store method : %v", err)
	}
	defer db.Close()
	svc := kv.NewServer(db, *nodeID, rn)

	grpcServer := grpc.NewServer()
	api.RegisterKVServer(grpcServer, svc)

	fmt.Printf("KV server listening on %s (node=%s)\n", *addr, *nodeID)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
