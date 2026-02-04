package kv

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Sjk4824/distributed_kv_store/storage"
)

type DurableStore struct {
	store        *Store
	wal          *storage.WAL
	walPath      string
	snapshotPath string

	snapEvery time.Duration
	stopCh    chan struct{}
	doneCh    chan struct{}
	mu        sync.Mutex
	keepval   int
}

func OpenDurableStore(walPath string, snapEvery time.Duration) (*DurableStore, error) {
	if dir := filepath.Dir(walPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	dir := filepath.Dir(walPath)
	snapshotPath := filepath.Join(dir, "snapshot.bin")

	s := NewStore()

	//we need to load the snapshot first
	state, err := storage.ReadSnapshot(snapshotPath)
	if err != nil {
		return nil, err
	}

	if state != nil {
		for k, v := range state {
			s.ForcePut(k, v)
		}
	}

	if err := storage.Replay(walPath, func(rec storage.Record) error {
		switch rec.Op {
		case storage.OpPut:
			s.ForcePut(rec.Key, rec.Value)
		case storage.OpDel:
			s.ForceDelete(rec.Key)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	w, err := storage.Open(walPath)
	if err != nil {
		return nil, err
	}

	ds := &DurableStore{
		store:        s,
		wal:          w,
		walPath:      walPath,
		snapshotPath: snapshotPath,
	}

	if snapEvery > 0 {
		ds.snapEvery = snapEvery
		ds.keepval = 3
		ds.stopCh = make(chan struct{})
		ds.doneCh = make(chan struct{})
		go ds.snapshotLoop()
	}
	return ds, nil
}

func (ds *DurableStore) Close() error {
	if ds.stopCh != nil {
		close(ds.stopCh)
		<-ds.doneCh
	}
	return ds.wal.Close()
}

func (ds *DurableStore) Get(key string) ([]byte, bool) {
	return ds.store.Get(key)
}

func (ds *DurableStore) Put(clientID string, reqId uint64, key string, val []byte) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if !ds.store.MarkSeen(clientID, reqId) {
		return nil
	}

	//if the wal append fails, we need to unmark the seen so that the request can be retried.
	if err := ds.wal.Append(storage.Record{Op: storage.OpPut, Key: key, Value: val}); err != nil {
		ds.store.UnmarkSeen(clientID, reqId)
		return err
	}
	ds.store.ForcePut(key, val)
	return nil
}

func (ds *DurableStore) Delete(clientID string, reqId uint64, key string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if !ds.store.MarkSeen(clientID, reqId) {
		return nil
	}

	if err := ds.wal.Append(storage.Record{Op: storage.OpDel, Key: key}); err != nil {
		ds.store.UnmarkSeen(clientID, reqId)
		return err
	}

	ds.store.ForceDelete(key)
	return nil
}
