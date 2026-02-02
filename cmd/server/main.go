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
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen %s: %v", *addr, err)
	}

	store := kv.NewStore()
	svc := kv.NewServer(store, *nodeID)

	grpcServer := grpc.NewServer()
	api.RegisterKVServer(grpcServer, svc)

	fmt.Printf("KV server listening on %s (node=%s)\n", *addr, *nodeID)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
