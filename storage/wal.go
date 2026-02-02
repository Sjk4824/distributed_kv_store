package storage

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sync"
)

type Op byte

const (
	OpPut Op = 1
	OpDel Op = 2
)

type Record struct {
	Op    Op
	Key   string
	Value []byte
}

type WAL struct {
	mu sync.Mutex
	f  *os.File
	w  *bufio.Writer
}

func Open(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		_ = f.Close()
		return nil, err
	}

	return &WAL{
		f: f,
		w: bufio.NewWriterSize(f, 1<<20),
	}, nil
}

func (l *WAL) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.f == nil {
		return nil
	}

	if err := l.w.Flush(); err != nil {
		_ = l.f.Close()
		l.f = nil
		return err
	}

	err := l.f.Close()
	l.f = nil
	return err
}

func (l *WAL) Append(rec Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.f == nil {
		return errors.New("wal is closed")
	}

	//write the operation byte here
	if err := l.w.WriteByte(byte(rec.Op)); err != nil {
		return err
	}

	//write the length of the key here
	keyB := []byte(rec.Key)
	if err := binary.Write(l.w, binary.LittleEndian, uint32(len(keyB))); err != nil {
		return err
	}

	//write the actual  key here
	if _, err := l.w.Write(keyB); err != nil {
		return err
	}

	valB := rec.Value
	if rec.Op == OpDel {
		valB = nil
	}
	//write the length of the value here
	if err := binary.Write(l.w, binary.LittleEndian, uint32(len(valB))); err != nil {
		return err
	}

	//write the actual value here
	if len(valB) > 0 {
		if _, err := l.w.Write(valB); err != nil {
			return err
		}
	}

	// Basically all the above writes were happening to the go buffer, we need to "flush" or provide it to the OS to actually write to the disk for persistance.
	if err := l.w.Flush(); err != nil {
		return err
	}
	//The sync method asks the OS to force all pending writes for this file onto a physical storage -> HDD/SSD etc.
	return l.f.Sync()
}

func Replay(path string, apply func(Record) error) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 1<<20)
	for {
		opB, err := r.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if Op(opB) != OpPut && Op(opB) != OpDel {
			return errors.New("invalid operation in wal")
		}

		var keyLen uint32
		if err := binary.Read(r, binary.LittleEndian, &keyLen); err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}

		keyB := make([]byte, keyLen)
		if _, err := io.ReadFull(r, keyB); err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}

		var valLen uint32
		if err := binary.Read(r, binary.LittleEndian, &valLen); err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}

		var val []byte
		if valLen > 0 {
			val = make([]byte, valLen)
			if _, err := io.ReadFull(r, val); err != nil {
				if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
					return nil
				}
				return err
			}
		}

		rec := Record{
			Op:    Op(opB),
			Key:   string(keyB),
			Value: val,
		}

		if err := apply(rec); err != nil {
			return nil
		}

	}
}
