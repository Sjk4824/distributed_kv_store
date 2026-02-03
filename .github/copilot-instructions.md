# Copilot instructions for contributors and AI agents

This file captures the minimal, high-value knowledge an AI coding assistant needs to be productive in this repository.

- **Big picture:** This is a small durable key-value service with a write-ahead log (WAL). The `cmd/server` binary runs a gRPC server exposing the API in `api/kv.proto`. Persistence is implemented in `kv` and `storage` packages via an append-only WAL and an in-memory `Store` that is reconstructed at startup.

- **Primary entrypoints:**
  - Server: [cmd/server/main.go](cmd/server/main.go)
  - Client scaffold: [cmd/client/main.go](cmd/client/main.go)
  - gRPC API: [api/kv.proto](api/kv.proto)

- **Core components & responsibilities:**
  - `kv/store.go` — in-memory key/value store with idempotency via `clientID:requestID` tracking (`seen` map). Use `ApplyPut`/`ApplyDelete` for request processing and `ForcePut`/`ForceDelete` when recovering from WAL.
  - `kv/durable_store.go` — coordinates WAL persistence + in-memory store. Important pattern: `MarkSeen` -> `wal.Append` -> `ForcePut/ForceDelete`; on WAL error it calls `UnmarkSeen` so the request can be retried.
  - `storage/wal.go` — WAL format and replay logic. Any changes to record layout MUST update both `Append` and `Replay` together.
  - `kv/grpc_server.go` — gRPC handlers; they validate input and delegate to `DurableStore`.

- **Concurrency & correctness notes:**
  - `Store` uses `sync.RWMutex` for reads/writes. WAL (`storage.WAL`) serializes file writes with its own mutex and calls `Sync()` after append.
  - Idempotency is handled in `Store` by generating a dedupe key `clientID:reqID` (see `dedupeKey` in `kv/store.go`).

- **Testing & common commands:**
  - Run unit tests: `go test ./...` (there are tests around WAL: `storage/wal_test.go`).
  - Run the server locally: `go run ./cmd/server -addr :50051 -wal ./data/wal.log -node node-1`.
  - Quick RPC checks: use `grpcurl` against `127.0.0.1:50051` to call `KV.Put`, `KV.Get`, and `KV.Delete` if the client is incomplete.

- **Project-specific conventions:**
  - WAL-first durability: persistent write happens before mutating the in-memory state (see `DurableStore.Put/Delete`). Do not invert this ordering.
  - Replay + Force*: replay uses `ForcePut`/`ForceDelete` to avoid marking requests as seen during recovery.
  - Health RPC currently returns a static leader/term — the `raft/` directory exists but is empty; distributed leader election is not implemented yet.

- **When editing core persistence or RPC behavior, check these files together:**
  - `storage/wal.go`
  - `kv/durable_store.go`
  - `kv/store.go`
  - `kv/grpc_server.go`

- **Examples to reference in PRs:**
  - To add a new field to WAL records: update `storage.Record`, adjust `Append` (encoding) and `Replay` (decoding), and update recovery logic in `kv/durable_store.go`.
  - To add a new RPC: add to `api/kv.proto`, regenerate protobufs (`protoc` with the Go plugin) and implement the handler in `kv/grpc_server.go`.

- **Developer workflow notes:**
  - Generated protobufs live under `api/` (`kv.pb.go` and `kv_grpc.pb.go`). If you change `api/kv.proto` run the usual `protoc --go_out=... --go-grpc_out=...` command matching project imports.
  - There is no cluster/raft behavior yet — changes that assume leader election must also add or modify `raft/` and the `Health` RPC semantics.

- If any of these areas are unclear or you'd like more precise examples (requests, expected WAL bytes, or test commands), tell me which section to expand.
