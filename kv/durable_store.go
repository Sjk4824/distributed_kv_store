package kv

import (
	"os"
	"path/filepath"

	"github.com/Sjk4824/distributed_kv_store/storage"
)

type DurableStore struct {
	store   *Store
	wal     *storage.WAL
	walPath string
}

func OpenDurableStore(walPath string) (*DurableStore, error) {
	if dir := filepath.Dir(walPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	s := NewStore()

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

	return &DurableStore{
		store:   s,
		wal:     w,
		walPath: walPath,
	}, nil
}
func (ds *DurableStore) Close() error {
	return ds.wal.Close()
}

func (ds *DurableStore) Get(key string) ([]byte, bool) {
	return ds.store.Get(key)
}

func (ds *DurableStore) Put(clientID string, reqId uint64, key string, val []byte) error {
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
