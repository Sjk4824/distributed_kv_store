package kv

import (
	"time"

	"github.com/Sjk4824/distributed_kv_store/storage"
)

func (ds *DurableStore) snapshotLoop() {
	ticker := time.NewTicker(ds.snapEvery)
	defer ticker.Stop()
	defer close(ds.doneCh)

	//this is the infinite loop which will run until we receive from stopCH
	for {
		select {
		case <-ticker.C:
			_ = ds.takeSnapshotOnce()
		case <-ds.stopCh:
			return
		}
	}
}

func (ds *DurableStore) takeSnapshotOnce() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	state := ds.store.Snapshot()
	return storage.WriteSnapshot(ds.snapshotPath, state)
}
