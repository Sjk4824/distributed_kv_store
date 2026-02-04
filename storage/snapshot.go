package storage

import (
	"encoding/gob"
	"os"
	"path/filepath"
)

func WriteSnapshot(path string, state map[string][]byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	enc := gob.NewEncoder(f)
	if err := enc.Encode(state); err != nil {
		_ = f.Close()
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	if dir := filepath.Dir(path); dir != "." {
		_ = os.Mkdir(dir, 0o755)
	}

	return os.Rename(tmp, path)
}

func ReadSnapshot(path string) (map[string][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	dec := gob.NewDecoder(f)
	var state map[string][]byte
	if err := dec.Decode(&state); err != nil {
		return nil, err
	}
	return state, nil
}
