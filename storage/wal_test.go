package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWALAppendAndReplay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := w.Append(Record{Op: OpPut, Key: "a", Value: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Op: OpPut, Key: "b", Value: []byte("y")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Op: OpDel, Key: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	err = Replay(path, func(r Record) error {
		switch r.Op {
		case OpPut:
			got[r.Key] = string(r.Value)
		case OpDel:
			delete(got, r.Key)
		default:
			t.Fatalf("unknown op: %v", r.Op)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := got["a"]; ok {
		t.Fatalf("expected a deleted")
	}
	if got["b"] != "y" {
		t.Fatalf("expected b=y, got %q", got["b"])
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
