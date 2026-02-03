package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Sjk4824/distributed_kv_store/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  client [flags] put <key> <value>
  client [flags] get <key>
  client [flags] del <key>
  client [flags] health

Flags:
`)
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:50051", "server address")
	clientID := flag.String("client", "cli", "client_id for idempotency")
	reqID := flag.Uint64("req", uint64(time.Now().UnixNano()), "request_id for idempotency")
	timeout := flag.Duration("timeout", 2*time.Second, "rpc timeout")
	flag.Parse()

	if flag.NArg() < 1 {
		usage()
	}

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial %s : %v", *addr, err)
	}
	defer conn.Close()

	c := api.NewKVClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	switch flag.Arg(0) {
	case "put":
		if flag.NArg() != 3 {
			usage()
		}
		key := flag.Arg(1)
		val := []byte(flag.Arg(2))

		resp, err := c.Put(ctx, &api.PutRequest{
			ClientId:  *clientID,
			RequestId: *reqID,
			Key:       key,
			Value:     val,
		})
		if err != nil {
			log.Fatalf("Put : %v", err)
		}
		fmt.Printf("ok=%v leader=%q\n", resp.GetOk(), resp.GetLeader())

	case "get":
		if flag.NArg() != 2 {
			usage()
		}
		key := flag.Arg(1)

		resp, err := c.Get(ctx, &api.GetRequest{
			Key: key,
		})
		if err != nil {
			log.Fatalf("Get : %v", err)
		}
		if !resp.GetFound() {
			fmt.Printf("key %q not found\n", key)
			return
		}
		fmt.Printf("value=%q\n", string(resp.GetValue()))

	case "del":
		if flag.NArg() != 2 {
			usage()
		}
		key := flag.Arg(1)
		resp, err := c.Delete(ctx, &api.DeleteRequest{
			ClientId:  *clientID,
			RequestId: *reqID,
			Key:       key,
		})
		if err != nil {
			log.Fatalf("Delete : %v", err)
		}
		fmt.Printf("ok=%v leader=%q\n", resp.GetOk(), resp.GetLeader())

	case "health":
		resp, err := c.Health(ctx, &api.HealthRequest{})
		if err != nil {
			log.Fatalf("Health: %v", err)
		}
		fmt.Printf("node=%s leader=%v term=%d\n", resp.GetNodeId(), resp.GetIsLeader(), resp.GetTerm())

	default:
		usage()
	}
}
