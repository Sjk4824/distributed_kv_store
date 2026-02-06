# Distributed KV Store with Raft Consensus

A production-ready distributed key-value store built on the **Raft consensus algorithm**, providing strong consistency, fault tolerance, and automatic leader election across a cluster of nodes.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Key Features](#key-features)
- [Project Structure](#project-structure)
- [Core Concepts](#core-concepts)
  - [Raft Consensus](#raft-consensus)
  - [Log Replication](#log-replication)
  - [Consistency Model](#consistency-model)
  - [Idempotency](#idempotency)
- [Building & Running](#building--running)
  - [Prerequisites](#prerequisites)
  - [Build](#build)
  - [Single Node](#single-node)
  - [3-Node Cluster](#3-node-cluster)
- [API & Usage](#api--usage)
  - [Protocol Buffers Definitions](#protocol-buffers-definitions)
  - [Client Usage](#client-usage)
  - [CLI Examples](#cli-examples)
- [Testing](#testing)
  - [Unit Tests](#unit-tests)
  - [Integration Tests](#integration-tests)
  - [Manual Testing Guide](#manual-testing-guide)
  - [Failover Testing](#failover-testing)
  - [Idempotency Testing](#idempotency-testing)
- [Design Details](#design-details)
  - [Why Raft?](#why-raft)
  - [Write-Ahead Log (WAL)](#write-ahead-log-wal)
  - [State Machine Application](#state-machine-application)
  - [Leadership Guarantees](#leadership-guarantees)
- [Troubleshooting](#troubleshooting)
- [Future Enhancements](#future-enhancements)
- [References](#references)

---

## Overview

This is a **distributed, fault-tolerant key-value store** that uses the **Raft consensus algorithm** to keep data consistent across multiple nodes. Whether one or two nodes fail, the cluster continues operating as long as a majority remains.

**Key guarantee:** All writes are replicated to a majority of nodes before being applied, ensuring:
- ✅ No data loss (even if nodes crash)
- ✅ Strong consistency (all reads see the same data)
- ✅ Automatic failover (new leader elected in ~3 seconds)
- ✅ Idempotent operations (duplicate requests don't double-apply)

---

## Architecture

### High-Level Overview

```
┌─────────────────────────────────────────────┐
│         gRPC API (KV Service)               │
│  Put(key, value)  Get(key)  Delete(key)     │
└────────┬────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────┐
│      KV Server (kv/grpc_server.go)          │
│   - Routes to Raft leader                   │
│   - Creates Commands for log replication    │
│   - Applies committed entries               │
└────────┬────────────────────────────────────┘
         │
         ├──────────────────────────┐
         │                          │
         ▼                          ▼
┌────────────────────┐  ┌──────────────────────┐
│   DurableStore     │  │   Raft Consensus    │
│  (kv/*)            │  │   (raft/*)          │
│                    │  │                      │
│ ┌────────────────┐ │  │ ┌────────────────┐  │
│ │  In-Memory     │ │  │ │  Raft Log      │  │
│ │  Store         │ │  │ │  (replicated)  │  │
│ │  (with mutex)  │ │  │ └────────────────┘  │
│ └────────────────┘ │  │                      │
│                    │  │ ┌────────────────┐  │
│ ┌────────────────┐ │  │ │  Leader Elect  │  │
│ │  WAL           │ │  │ │  + Voting      │  │
│ │  (durability)  │ │  │ └────────────────┘  │
│ └────────────────┘ │  │                      │
│                    │  │ ┌────────────────┐  │
│  - MarkSeen       │  │ │  Replication   │  │
│  - ApplyPut       │  │ │  to Followers  │  │
│  - ApplyDelete    │  │ └────────────────┘  │
└────────────────────┘  └──────────────────────┘
```

### Replication Flow

```
Client Request
      │
      ▼
  [Leader]
      │
      ├─ AppendLog() ──► Raft Log
      │
      ├─ replicateToPeer() ──► [Follower 1] + [Follower 2]
      │
      └─► updateCommitIndex() ──► Majority has entry?
                                        │
                                        ├─ YES: commitIndex++
                                        │
                                        └─► ApplyEntries()
                                                  │
                                                  ▼
                                            DurableStore
                                                  │
                                                  ▼
                                            Response to Client
```

---

## Key Features

| Feature | Description |
|---------|-------------|
| **Raft Consensus** | Automatic leader election & log replication |
| **Strong Consistency** | Reads always see committed data |
| **Fault Tolerance** | Tolerates up to ⌊N/2⌋ node failures |
| **Durability** | Write-ahead log ensures no data loss |
| **Idempotency** | Duplicate requests tracked by clientID:requestID |
| **Automatic Failover** | New leader elected within ~3 seconds |
| **gRPC API** | Type-safe, efficient RPC protocol |
| **Distributed** | Scales to 3+ nodes with custom peer lists |

---

## Project Structure

```
distributed_kv_store/
├── README.md                          # This file
├── go.mod / go.sum                    # Go module definitions
├── .github/
│   └── copilot-instructions.md        # AI agent guidelines
│
├── api/                               # Protocol Buffers & Generated Code
│   ├── kv.proto                       # KV service definitions
│   ├── raft.proto                     # Raft RPC definitions
│   ├── kv.pb.go / kv_grpc.pb.go       # Generated KV code
│   └── raft.pb.go / raft_grpc.pb.go   # Generated Raft code
│
├── kv/                                # Key-Value Store Implementation
│   ├── store.go                       # In-memory store with idempotency tracking
│   ├── durable_store.go               # WAL coordination + state machine
│   └── grpc_server.go                 # KV service gRPC handler
│
├── raft/                              # Raft Consensus Implementation
│   ├── node.go                        # Core Raft node logic
│   │   ├── Leader Election (startElection)
│   │   ├── Replication (replicateToPeer, updateCommitIndex)
│   │   └── Application (ApplyEntries)
│   └── rpc.go                         # RPC handlers (RequestVote, AppendEntries)
│
├── storage/                           # Persistence Layer
│   ├── wal.go                         # Write-Ahead Log implementation
│   ├── wal_test.go                    # WAL unit tests
│   └── snapshot.go                    # (Optional) snapshot support
│
├── cmd/                               # Executables
│   ├── server/main.go                 # Multi-node server with Raft
│   └── client/main.go                 # CLI client for testing
│
├── data/                              # Runtime data (created at startup)
│   ├── node1/wal.log                  # Node 1 WAL
│   ├── node2/wal.log                  # Node 2 WAL
│   └── node3/wal.log                  # Node 3 WAL
│
└── tests/
    └── chaos/                         # (Optional) chaos testing
```

---

## Core Concepts

### Raft Consensus

Raft is a consensus algorithm that solves the problem: *"How do we keep multiple nodes synchronized even when some fail?"*

**Three key roles:**

1. **Leader** — Single source of truth, accepts writes, replicates to followers
2. **Follower** — Passively receives log entries from leader, applies when committed
3. **Candidate** — Temporarily becomes candidate during election if no heartbeat received

**Election Process:**

```
Follower (no heartbeat for 1.5-3s)
    │
    ▼
Candidate (term++, request votes)
    │
    ├─ Majority votes? ──► YES ──► Leader ✓
    │
    └─ Higher term seen? ──► YES ──► Step down to Follower
```

### Log Replication

The core mechanism: **replicate-then-apply**

```
1. Client sends Put(key, value) to Leader
2. Leader appends to its log: [Entry{term: 5, index: 10, cmd: Put(...)}]
3. Leader sends log entries to all Followers
4. Followers append entries to their logs (but don't apply yet)
5. Leader waits for majority acknowledgment (e.g., 2 out of 3)
6. Leader advances commitIndex to 10
7. All nodes apply entry at index 10 to their state machines
8. Response sent back to client
```

**Why this works:**
- If leader crashes before commit, entry will be overwritten by new leader's log
- If follower crashes, it replays from WAL on restart
- Majority guarantee = at least one replica has the entry

### Consistency Model

**This system provides:**
- ✅ **Strong Consistency** — All nodes eventually have same state
- ✅ **Linearizability** — Operations appear to execute in real-time order
- ✅ **Durability** — Committed writes survive node failures

**What you get:**

| Operation | Guarantee |
|-----------|-----------|
| **Put** | Leader writes entry, replicates to majority, returns success |
| **Get** | Reads from any node (reads see committed state) |
| **Delete** | Same as Put — replicated then applied |

### Idempotency

Prevents duplicate applies when requests are retried:

```go
// Each request has a unique (clientID, requestID)
seen := map[string]struct{}{}  // clientID:requestID → applied

// On each request:
dedupeKey := fmt.Sprintf("%s:%d", clientID, requestID)
if _, alreadySeen := seen[dedupeKey]; alreadySeen {
    return "already applied"  // Don't apply again
}

// Mark as seen BEFORE persisting
seen[dedupeKey] = struct{}{}
```

This ensures:
- First request with (A, 123) → applied
- Retry with same (A, 123) → returns cached result, no double-apply
- Different requestID → always applied

---

## Building & Running

### Prerequisites

- **Go** 1.20 or later
- **Protocol Buffers** compiler (optional; `.pb.go` files already generated)
- **gRPC** libraries (auto-installed via `go mod`)

### Build

```bash
cd /Users/sadhanajayakumar/Desktop/distributed_kv_store

# Download dependencies
go mod download

# Build server and client
go build -o server ./cmd/server
go build -o client ./cmd/client
```

### Single Node

Quick test with a single node (no replication):

```bash
mkdir -p data/single
./server \
  -node single-node \
  -addr 127.0.0.1:50051 \
  -raft_addr 127.0.0.1:60051 \
  -peers ""
```

Then in another terminal:
```bash
./client -addr 127.0.0.1:50051 put mykey myvalue
./client -addr 127.0.0.1:50051 get mykey
```

### 3-Node Cluster

The recommended setup for testing fault tolerance.

**Terminal 1 — Node 1:**
```bash
mkdir -p data/node{1,2,3}
./server \
  -node node-1 \
  -addr 127.0.0.1:50051 \
  -raft_addr 127.0.0.1:60051 \
  -wal ./data/node1/wal.log \
  -peers 127.0.0.1:60052,127.0.0.1:60053
```

**Terminal 2 — Node 2:**
```bash
./server \
  -node node-2 \
  -addr 127.0.0.1:50052 \
  -raft_addr 127.0.0.1:60052 \
  -wal ./data/node2/wal.log \
  -peers 127.0.0.1:60051,127.0.0.1:60053
```

**Terminal 3 — Node 3:**
```bash
./server \
  -node node-3 \
  -addr 127.0.0.1:50053 \
  -raft_addr 127.0.0.1:60053 \
  -wal ./data/node3/wal.log \
  -peers 127.0.0.1:60051,127.0.0.1:60052
```

**Terminal 4 — Client:**
```bash
# Check health (one should be leader)
./client -addr 127.0.0.1:50051 health
./client -addr 127.0.0.1:50052 health
./client -addr 127.0.0.1:50053 health

# Write data
./client -addr 127.0.0.1:50051 put key1 value1

# Read from all nodes (should be consistent)
./client -addr 127.0.0.1:50051 get key1
./client -addr 127.0.0.1:50052 get key1
./client -addr 127.0.0.1:50053 get key1
```

---

## API & Usage

### Protocol Buffers Definitions

#### KV Service (api/kv.proto)

```protobuf
service KV {
  rpc Put(PutRequest) returns (PutResponse);
  rpc Get(GetRequest) returns (GetResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
  rpc Health(HealthRequest) returns (HealthResponse);
}

message PutRequest {
  string key = 1;
  bytes value = 2;
  string client_id = 3;        // For idempotency
  uint64 request_id = 4;       // For idempotency
}

message PutResponse {
  bool ok = 1;
  string leader = 2;           // Hint for retry
}

message GetRequest {
  string key = 1;
}

message GetResponse {
  bool found = 1;
  bytes value = 2;
  string leader = 3;
}

message DeleteRequest {
  string key = 1;
  string client_id = 2;
  uint64 request_id = 3;
}

message DeleteResponse {
  bool ok = 1;
  string leader = 2;
}

message HealthRequest {}

message HealthResponse {
  string node_id = 1;
  bool is_leader = 2;
  uint64 term = 3;             // Raft term for lease
}
```

#### Raft Service (api/raft.proto)

```protobuf
service Raft {
  rpc RequestVote(RequestVoteRequest) returns (RequestVoteResponse);
  rpc AppendEntries(AppendEntriesRequest) returns (AppendEntriesResponse);
}

message LogEntry {
  uint64 term = 1;             // Term when entry was received
  uint64 index = 2;            // Index in the log
  Command cmd = 3;             // The actual command (Put/Delete)
}

message Command {
  string client_id = 1;
  uint64 request_id = 2;
  oneof op {
    Put put = 3;
    Delete del = 4;
  }
}

message AppendEntriesRequest {
  string leader_id = 1;
  uint64 term = 2;
  uint64 prev_log_index = 3;   // For log matching check
  uint64 prev_long_term = 4;   // Term of previous entry
  repeated LogEntry entries = 5;
  uint64 leader_commit = 6;    // Leader's commitIndex
}
```

### Client Usage

#### Go Client (Programmatic)

```go
package main

import (
    "context"
    "log"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "github.com/Sjk4824/distributed_kv_store/api"
)

func main() {
    conn, err := grpc.NewClient(
        "127.0.0.1:50051",
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := api.NewKVClient(conn)
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    // Put
    resp, err := client.Put(ctx, &api.PutRequest{
        Key:       "mykey",
        Value:     []byte("myvalue"),
        ClientId:  "client1",
        RequestId: 123,
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Put response: ok=%v, leader=%q\n", resp.GetOk(), resp.GetLeader())

    // Get
    resp2, err := client.Get(ctx, &api.GetRequest{Key: "mykey"})
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Get response: found=%v, value=%q\n", resp2.GetFound(), string(resp2.GetValue()))
}
```

### CLI Examples

#### Health Check

```bash
./client -addr 127.0.0.1:50051 health
# Output: node_id=node-1 is_leader=true term=5

./client -addr 127.0.0.1:50052 health
# Output: node_id=node-2 is_leader=false term=5
```

#### Write Operations

```bash
# Put a key-value pair
./client -addr 127.0.0.1:50051 put username john_doe
# Output: ok=true leader=""

# Delete a key
./client -addr 127.0.0.1:50051 del username
# Output: ok=true leader=""

# Retry with same requestID (idempotent)
./client -addr 127.0.0.1:50051 -client myclient -req 100 put key value
./client -addr 127.0.0.1:50051 -client myclient -req 100 put key value
# Both return success, but only applied once
```

#### Read Operations

```bash
# Get a value
./client -addr 127.0.0.1:50051 get username
# Output: value="john_doe"

# Get non-existent key
./client -addr 127.0.0.1:50051 get nonexistent
# Output: key "nonexistent" not found

# Read from follower (still consistent)
./client -addr 127.0.0.1:50052 get username
# Output: value="john_doe"
```

---

## Testing

### Unit Tests

Run the WAL tests:

```bash
go test ./storage -v
```

Expected output:
```
=== RUN   TestWALAppendAndReplay
--- PASS: TestWALAppendAndReplay (0.01s)
PASS
ok      github.com/Sjk4824/distributed_kv_store/storage 0.164s
```

### Integration Tests

Build and run a 3-node cluster, then test manually (see [Manual Testing Guide](#manual-testing-guide)).

### Manual Testing Guide

**Start the 3-node cluster** (as shown in [3-Node Cluster](#3-node-cluster))

#### Test 1: Data Replication

```bash
# Write to leader
./client -addr 127.0.0.1:50051 put test_key test_value

# Read from all nodes
./client -addr 127.0.0.1:50051 get test_key
# Output: value="test_value"

./client -addr 127.0.0.1:50052 get test_key
# Output: value="test_value"

./client -addr 127.0.0.1:50053 get test_key
# Output: value="test_value"
```

✅ **Success**: All nodes have the same value

#### Test 2: Leader Election

```bash
# Check who is leader
./client -addr 127.0.0.1:50051 health
./client -addr 127.0.0.1:50052 health
./client -addr 127.0.0.1:50053 health

# One should show is_leader=true
```

#### Test 3: Consistency Under Failures

```bash
# Kill the leader (Ctrl+C in its terminal)
# Within ~3 seconds, run health check
./client -addr 127.0.0.1:50052 health
./client -addr 127.0.0.1:50053 health

# One of the remaining 2 should be new leader

# Try to write to new leader
./client -addr 127.0.0.1:50052 put new_key new_value

# Verify all remaining nodes have it
./client -addr 127.0.0.1:50052 get new_key
./client -addr 127.0.0.1:50053 get new_key
```

✅ **Success**: System continues working with 2/3 nodes

#### Test 4: Multi-Key Operations

```bash
# Write multiple keys
./client -addr 127.0.0.1:50051 put user:1 alice
./client -addr 127.0.0.1:50051 put user:2 bob
./client -addr 127.0.0.1:50051 put user:3 charlie

# Read from different node
./client -addr 127.0.0.1:50052 get user:1
./client -addr 127.0.0.1:50052 get user:2
./client -addr 127.0.0.1:50053 get user:3
```

✅ **Success**: All keys replicated and consistent

### Failover Testing

**Scenario: Kill leader, verify automatic failover**

```bash
# Terminal 1: Start 3 nodes
# (Run all 3 server commands)

# Terminal 4: Find leader
./client -addr 127.0.0.1:50051 health  # Check this
# Output: is_leader=true  → This is leader

# Terminal 4: Kill the leader
# Press Ctrl+C in the terminal running node-1

# Terminal 4: Verify failover
./client -addr 127.0.0.1:50052 health  # Try other nodes
./client -addr 127.0.0.1:50053 health

# One should show is_leader=true

# Write to new leader
./client -addr 127.0.0.1:50052 put failover_key success

# Restart old leader
# Go back to its terminal and run the same command

# Old leader should rejoin cluster and catch up
```

### Idempotency Testing

**Scenario: Same request applied only once**

```bash
# Send request with fixed clientID and requestID
./client \
  -addr 127.0.0.1:50051 \
  -client my_client \
  -req 999 \
  put idempotent_key value1

# Retry same request 3 times
./client \
  -addr 127.0.0.1:50051 \
  -client my_client \
  -req 999 \
  put idempotent_key value1

./client \
  -addr 127.0.0.1:50051 \
  -client my_client \
  -req 999 \
  put idempotent_key value1

# All 3 return success, but value is "value1" (applied once)

# Try different requestID
./client \
  -addr 127.0.0.1:50051 \
  -client my_client \
  -req 1000 \
  put idempotent_key value2

# Now value is "value2" (new request applied)
```

---

## Design Details

### Why Raft?

**Problem solved:**
- ❌ Single-node KV store = data loss if node crashes
- ❌ Naive replication = inconsistency if leader fails
- ✅ Raft = consensus = automatic leader election + log replication

**Raft advantages:**
1. **Understandable** — Simpler than Paxos, well-documented
2. **Safe** — Guarantees safety under all conditions
3. **Lively** — Eventually makes progress even during failures
4. **Proven** — Used by etcd, Consul, InfluxDB

### Write-Ahead Log (WAL)

**Purpose:** Durability without slow disk syncs

**How it works:**

```
Client request:
1. Append to WAL file (disk write)
2. Flush WAL buffer
3. Sync to disk (fsync)
4. Apply to in-memory state
5. Return to client
```

**Format** (binary, little-endian):

```
[OpByte][KeyLen(4B)][Key][ValueLen(4B)][Value]
```

Example for `Put("hello", "world")`:
```
01                    # OpPut
05 00 00 00           # KeyLen = 5
68 65 6c 6c 6f        # "hello"
05 00 00 00           # ValueLen = 5
77 6f 72 6c 64        # "world"
```

**Recovery:** On startup, replay WAL entries to reconstruct state:

```go
Replay(walPath, func(rec Record) {
    switch rec.Op {
    case OpPut:
        store.ForcePut(rec.Key, rec.Value)
    case OpDel:
        store.ForceDelete(rec.Key)
    }
})
```

### State Machine Application

**Three-phase apply pipeline:**

```
┌─────────────────────────────────────────┐
│ 1. Raft Replication Phase               │
│ - Leader: replicate to followers        │
│ - Followers: store in log (not applied) │
└─────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│ 2. Commit Phase                         │
│ - Leader: wait for majority             │
│ - Leader: advance commitIndex           │
│ - Followers: apply when leader tells    │
└─────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│ 3. State Machine Apply Phase            │
│ - All nodes: apply [lastApplied+1 ...]  │
│             to [commitIndex]            │
│ - Update: key-value store               │
│ - Update: lastApplied++                 │
└─────────────────────────────────────────┘
```

Code flow:

```go
// Leader advances commitIndex (updateCommitIndex)
if majorityHasEntry {
    commitIndex = entryIndex
}

// Apply loop (every 50ms)
ApplyEntries(func(entry *LogEntry) {
    cmd := entry.Cmd
    if cmd.Op == Put {
        store.ForcePut(cmd.Key, cmd.Value)
    } else if cmd.Op == Delete {
        store.ForceDelete(cmd.Key)
    }
    lastApplied++
})
```

### Leadership Guarantees

**Why this system is safe:**

1. **Election Safety** — At most one leader per term
   - Each node votes only once per term
   - Majority must vote for leader

2. **Log Matching** — If two nodes have same index, entries are identical
   - New leader includes prevLogIndex & prevLogTerm check
   - Follower only accepts if log matches

3. **Leader Completeness** — Leader has all committed entries
   - Only nodes with up-to-date logs can become leader
   - `uptoDate()` check: compare lastLogTerm & lastLogIndex

4. **State Machine Safety** — Each command applied exactly once
   - Only committed entries applied
   - Idempotency via clientID:requestID tracking

---

## Troubleshooting

### Issue: Nodes can't communicate

**Symptom:** Nodes running but no leader elected (all Followers)

**Debug:**
```bash
# Check all 3 nodes starting
# Look for "[raft node-X] became LEADER"

# If not appearing, check:
# 1. Are peer addresses correct?
./server -node node-1 ... -peers 127.0.0.1:60052,127.0.0.1:60053

# 2. Are ports available?
lsof -i :60051  # Check if port 60051 is in use
```

**Fix:**
- Verify peer list matches actual node raft_addr
- Ensure no firewall blocking ports 60051-60053
- Try localhost addresses (127.0.0.1) instead of hostname

### Issue: Writes to follower fail

**Symptom:**
```bash
./client -addr 127.0.0.1:50052 put key value
# Returns: ok=false leader="127.0.0.1:60051"
```

**Why:** Followers reject writes, only leader accepts

**Fix:** Write to leader address:
```bash
./client -addr 127.0.0.1:50051 put key value
```

Or implement client-side smart retry using the `leader` hint from response.

### Issue: Data inconsistency after restart

**Symptom:** Data lost or inconsistent after node restart

**Why:** WAL file not flushed to disk

**Debug:**
```bash
# Check WAL file exists
ls -la data/node1/wal.log

# Check file size (should be > 0 if entries added)
du -h data/node1/wal.log
```

**Fix:** WAL is already durable. If issue persists:
```bash
# Restart nodes one at a time
# First node starts fresh from WAL
./server -node node-1 ...

# Give it time to become leader (~3s)
# Then start other nodes to catch up
```

### Issue: Leader election takes too long

**Symptom:** Takes >5 seconds to elect new leader

**Tuning:** (in raft/node.go)
```go
electionMin: 1500 * time.Millisecond,  // Decrease to 500ms for faster
electionMax: 3000 * time.Millisecond,  // Decrease to 1000ms for faster
heartbeat:   100 * time.Millisecond,   // Increase to 200ms (must be < electionMin/10)
```

⚠️ Warning: Shorter timeouts = more elections under network latency

---

## Future Enhancements

### High Priority

- [ ] **Client-side retry logic** — Automatically retry on non-leader with leader hint
- [ ] **Snapshots** — Compact large logs to reduce startup time
- [ ] **Read lease optimization** — Serve reads without consensus (unsafe but fast)
- [ ] **Persistence verification** — Unit test WAL replay

### Medium Priority

- [ ] **Membership changes** — Add/remove nodes dynamically
- [ ] **Monitoring** — Metrics for replication lag, leader changes
- [ ] **Performance testing** — Benchmark throughput at different cluster sizes
- [ ] **Chaos testing** — Automated node kills & network delays

### Lower Priority

- [ ] **Multi-Raft (sharding)** — Split keyspace across multiple Raft groups
- [ ] **TLS support** — Encrypted inter-node communication
- [ ] **Admin API** — Force leader step-down, node removal
- [ ] **Lua scripting** — Atomic multi-key operations

---

## References

### Papers & Resources

1. **Raft Paper** — "In Search of an Understandable Consensus Algorithm"
   - https://raft.github.io/raft.pdf
   - Clear explanation of consensus, leader election, log replication

2. **Raft Visualization** — Interactive explanation
   - http://raftconsensus.github.io/

3. **Etcd Documentation** — Production Raft implementation
   - https://etcd.io/docs/v3.5/

### Tools Used

- **Protocol Buffers** — Type-safe RPC definitions
- **gRPC** — High-performance RPC framework
- **Go sync.Mutex** — Lock primitives for thread safety
- **Write-Ahead Log (WAL)** — Durability pattern

### Code Layout Rationale

| Component | Why This Location |
|-----------|-------------------|
| `raft/node.go` | Core Raft state machine |
| `raft/rpc.go` | Raft RPC handlers |
| `kv/store.go` | In-memory state, idempotency |
| `kv/durable_store.go` | WAL + Store coordination |
| `kv/grpc_server.go` | KV API + command logging |
| `cmd/server/main.go` | Cluster bootstrap |
| `storage/wal.go` | Binary log format |
| `api/*.proto` | Service definitions |

---

## Getting Help

**Issues:**
1. Check [Troubleshooting](#troubleshooting) section
2. Look at server logs (printed to terminal)
3. Verify 3-node cluster setup matches [3-Node Cluster](#3-node-cluster)

**Want to extend?**
- See [copilot-instructions.md](.github/copilot-instructions.md) for AI agent guidelines
- Follow patterns in existing code
- Test with 3-node cluster before deploying

---

**Last Updated:** February 6, 2026  
**Status:** ✅ Fully Functional — Tested with 3-node cluster
