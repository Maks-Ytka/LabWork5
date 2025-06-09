package datastore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const maxSegmentSize = 100
const outFileName = "current-data"
const getWorkerCount = 10

type writeRequest struct {
	entry entry
	done  chan struct{}
}

type getRequest struct {
	key      string
	response chan getResult
}

type getResult struct {
	value string
	err   error
}

type Db struct {
	segments []*segment
	current  *segment
	dir      string

	mu      sync.RWMutex
	putCh   chan writeRequest
	getCh   chan getRequest
	done    chan struct{}
	wg      sync.WaitGroup
	onceGet sync.Once
}

func Open(dir string) (*Db, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var segs []*segment
	for _, f := range files {
		if !f.Type().IsRegular() {
			continue
		}
		name := f.Name()
		if strings.HasPrefix(name, "segment-") || name == outFileName {
			s, err := newSegment(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		s, err := newSegment(filepath.Join(dir, outFileName))
		if err != nil {
			return nil, err
		}
		segs = append(segs, s)
	}

	sort.Slice(segs, func(i, j int) bool {
		return segs[i].path < segs[j].path
	})

	db := &Db{
		segments: segs,
		current:  segs[len(segs)-1],
		dir:      dir,
		putCh:    make(chan writeRequest, 128),
		getCh:    make(chan getRequest, 128),
		done:     make(chan struct{}),
	}

	db.wg.Add(1)
	go db.writer()

	db.onceGet.Do(func() {
		for i := 0; i < getWorkerCount; i++ {
			db.wg.Add(1)
			go db.reader()
		}
	})

	return db, nil
}

func (db *Db) writer() {
	defer db.wg.Done()
	for {
		select {
		case req := <-db.putCh:
			db.mu.Lock()
			offset := db.current.offset
			n, err := db.current.file.Write(req.entry.Encode())
			if err == nil {
				db.current.index[req.entry.key] = offset
				db.current.offset += int64(n)
			}
			if db.current.size() >= maxSegmentSize {
				newPath := filepath.Join(db.dir, fmt.Sprintf("segment-%d", len(db.segments)))
				newSeg, err := newSegment(newPath)
				if err == nil {
					db.segments = append(db.segments, newSeg)
					db.current = newSeg
				}
			}
			db.mu.Unlock()
			close(req.done)
		case <-db.done:
			return
		}
	}
}

func (db *Db) reader() {
	defer db.wg.Done()
	for {
		select {
		case req := <-db.getCh:
			db.mu.RLock()
			var val string
			var err error = ErrNotFound

			for i := len(db.segments) - 1; i >= 0; i-- {
				val, err = db.segments[i].get(req.key)
				if err == nil {
					break
				}
				if !errors.Is(err, ErrNotFound) {
					break
				}
			}
			db.mu.RUnlock()
			req.response <- getResult{value: val, err: err}
		case <-db.done:
			return
		}
	}
}

func (db *Db) Put(key, value string) error {
	req := writeRequest{
		entry: entry{key: key, value: value},
		done:  make(chan struct{}),
	}
	db.putCh <- req
	<-req.done
	return nil
}

func (db *Db) Get(key string) (string, error) {
	resp := make(chan getResult)
	db.getCh <- getRequest{key: key, response: resp}
	r := <-resp
	return r.value, r.err
}

func (db *Db) Size() (int64, error) {
	var total int64
	for _, seg := range db.segments {
		total += seg.size()
	}
	return total, nil
}

func (db *Db) Close() error {
	close(db.done)
	db.wg.Wait()
	for _, seg := range db.segments {
		_ = seg.close()
	}
	return nil
}
