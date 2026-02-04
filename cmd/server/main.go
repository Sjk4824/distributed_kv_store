package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/Sjk4824/distributed_kv_store/api"
	"github.com/Sjk4824/distributed_kv_store/kv"
	"google.golang.org/grpc"
)

func main() {
	addr := flag.String("addr", ":50051", "The address to listen on")
	nodeID := flag.String("node", "node-1", "node id")
	walPath := flag.String("wal", "./data/wal.log", "path to wal file")
	snapEvery := flag.Duration("snap_every", 0, "snapshot interval (e.g. 5s, 30s). 0 disables snapshots")

	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}
	db, err := kv.OpenDurableStore(*walPath, *snapEvery)
	if err != nil {
		log.Fatalf("open durable store method : %v", err)
	}
	defer db.Close()
	svc := kv.NewServer(db, *nodeID)

	grpcServer := grpc.NewServer()
	api.RegisterKVServer(grpcServer, svc)

	fmt.Printf("KV server listening on %s (node=%s)\n", *addr, *nodeID)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
