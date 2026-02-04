package kv

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

	if err := storage.WriteSnapshot(ds.snapshotPath, state); err != nil {
		return err
	}

	if err := ds.wal.Close(); err != nil {
		return err
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	rotated := fmt.Sprintf("%s.%s.old", ds.walPath, ts)

	if dir := filepath.Dir(ds.walPath); dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}

	if err := os.Rename(ds.walPath, rotated); err != nil {
		return err
	}

	w, err := storage.Open(ds.walPath)
	if err != nil {
		return err
	}
	ds.wal = w

	if ds.keepval > 0 {
		_ = cleanupOldWALs(ds.walPath, ds.keepval)
	}
	return nil
}

func cleanupOldWALs(walPath string, keep int) error {
	if keep <= 0 {
		return nil
	}

	dir := filepath.Dir(walPath)
	base := filepath.Base(walPath)

	prefix := base + "."
	suffix := ".old"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var rotatedFiles []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			rotatedFiles = append(rotatedFiles, filepath.Join(dir, name))
		}
	}

	sort.Strings(rotatedFiles)

	if len(rotatedFiles) <= keep {
		return nil
	}

	toDelete := rotatedFiles[:len(rotatedFiles)-keep]
	for _, file := range toDelete {
		_ = os.Remove(file)
	}
	return nil
}
