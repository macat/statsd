package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"os"
	"path"
	"sync"
)

// TODO: clean shutdown
// TODO: remove debug info

type fsDatastore struct {
	sync.Mutex
	sync.Cond
	dirName string
	streams map[string]*fsDsStream
	queue   []*fsDsStream
}

type fsDsStream struct {
	sync.Mutex
	name         string
	dirName      string
	tail         []Record
	dat, idx     *os.File
	valid        bool
	lastWr       int64
	dsize, isize int64
}

func NewFsDatastore(dirName string) Datastore {
	ds := &fsDatastore{
		dirName: path.Clean(dirName) + "/",
		streams: make(map[string]*fsDsStream),
	}
	ds.Cond.L = &ds.Mutex
	return ds
}

func (ds *fsDatastore) Init() error {
	// TODO
	go ds.write()
	return nil
}

func (ds *fsDatastore) Insert(name string, r Record) error {
	st := ds.getStream(name)
	st.tail = append(st.tail, r)
	st.Unlock()
	return nil
}

func (ds *fsDatastore) Query(name string, from, until int64) ([]Record, error) {
	// TODO
	return []Record{}, nil
}

func (ds *fsDatastore) LatestBefore(name string, ts int64) (Record, error) {
	// TODO
	return Record{}, ErrNoData
}

func (ds *fsDatastore) getStream(name string) *fsDsStream {
	ds.Lock()
	if _, ok := ds.streams[name]; !ok {
		st := &fsDsStream{
			name:    name,
			dirName: ds.dirName,
		}
		ds.streams[name] = st
		ds.queue = append(ds.queue, st)
		if len(ds.queue) == 1 {
			ds.Signal()
		}
		log.Println("loaded: ", name)
	}
	st := ds.streams[name]
	st.Lock()
	ds.Unlock()
	return st
}

func (ds *fsDatastore) write() {
	n := -1
	for {
		ds.Lock()
		if len(ds.queue) == 0 {
			ds.Wait()
		}
		l := len(ds.queue)
		if n++; n >= l {
			n = 0
		}
		st := ds.queue[n]
		st.Lock()
		if len(st.tail) == 0 {
			ds.queue[n] = ds.queue[l-1]
			ds.queue[l-1] = nil
			ds.queue = ds.queue[0 : l-1]
			delete(ds.streams, st.name)
			if cap(ds.queue) > 3*(l-1) {
				log.Println("queue shrink:", cap(ds.queue), l-1)
				x := make([]*fsDsStream, l-1, 2*(l-1))
				copy(x, ds.queue)
				ds.queue = x
			}
			st.Unlock()
			ds.Unlock()
			log.Println("delete:", st.name)
			continue
		} else {
			ds.Unlock()
			if err := st.writeTail(); err != nil {
				st.valid = false
				log.Println("write:", err)
			}
			if cap(st.tail) > 3*len(st.tail) {
				log.Println("tail shrink:", cap(st.tail), len(st.tail))
				st.tail = make([]Record, 0, 2*len(st.tail))
			} else {
				st.tail = st.tail[:0]
			}
			st.Unlock()
		}
	}
}

func (st *fsDsStream) fileName() string {
	return st.dirName + st.name
}

func (st *fsDsStream) writeTail() error {
	log.Println(st.fileName(), len(st.tail))
	if err := st.openFiles(); err != nil {
		return err
	}
	defer st.closeFiles()

	dbuff, ibuff := new(bytes.Buffer), new(bytes.Buffer)
	dsize, isize, lastWr := st.dsize, st.isize, st.lastWr

	for _, r := range st.tail {
		if r.Ts%60 != 0 {
			log.Println("Timestamp not divisible by 60")
			continue
		} else if lastWr >= r.Ts {
			log.Println("Timestamp in the past")
			continue
		}

		binary.Write(dbuff, binary.LittleEndian, r.Value)
		dsize += 8
		lastWr += 60

		if r.Ts > lastWr {
			binary.Write(ibuff, binary.LittleEndian, []int64{r.Ts, dsize - 8})
			isize += 16
			lastWr = r.Ts
		}
	}

	if _, err := st.dat.Seek(0, os.SEEK_END); err != nil {
		return err
	}
	if _, err := st.idx.Seek(0, os.SEEK_END); err != nil {
		return err
	}

	if _, err := dbuff.WriteTo(st.dat); err != nil {
		return err
	}
	if _, err := ibuff.WriteTo(st.idx); err != nil {
		return err
	}

	st.dsize, st.isize, st.lastWr = dsize, isize, lastWr
	return nil
}

func (st *fsDsStream) openFiles() error {
	dat, err := os.OpenFile(st.fileName()+".dat", os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}

	idx, err := os.OpenFile(st.fileName()+".idx", os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		dat.Close()
		return err
	}
	st.dat, st.idx = dat, idx

	if !st.valid {
		di, err := dat.Stat()
		if err != nil {
			dat.Close()
			idx.Close()
			return err
		}

		ii, err := idx.Stat()
		if err != nil {
			dat.Close()
			idx.Close()
			return err
		}
		st.dsize, st.isize = di.Size(), ii.Size()

		if st.isize == 0 {
			st.lastWr = -1 << 63
		} else {
			ts, pos, err := st.getIdxEntry((st.isize / 16) - 1)
			if err != nil {
				dat.Close()
				idx.Close()
				return err
			}
			st.lastWr = ts + 60*((st.dsize-pos)/8-1)
		}
		st.valid = true
	}
	return nil
}

func (st *fsDsStream) closeFiles() {
	if st.dat != nil {
		if err := st.dat.Sync(); err != nil {
			log.Println("closeFiles:", err)
		}
		st.dat.Close()
		st.dat = nil
	}
	if st.idx != nil {
		if err := st.idx.Sync(); err != nil {
			log.Println("closeFiles:", err)
		}
		st.idx.Close()
		st.idx = nil
	}
}

func (st *fsDsStream) getIdxEntry(n int64) (ts int64, pos int64, err error) {
	if _, err := st.idx.Seek(16*n, os.SEEK_SET); err != nil {
		return 0, 0, err
	}
	data := []int64{0, 0}
	if err := binary.Read(st.idx, binary.LittleEndian, data); err != nil {
		return 0, 0, err
	}
	return data[0], data[1], nil
}

func (ds *fsDatastore) hashName(name string) string {
	var x uint64
	for _, ch := range name {
		for i := 15; i >= 0; i-- {
			x <<= 1
			x ^= 0x1edc6f41 * ((x >> 32) ^ (uint64(ch)>>uint(i))&1)
		}
	}
	x &= 0xffff
	x %= 1000

	s := []byte{'0' + byte(x/100), '0' + byte((x/10)%10), '0' + byte(x%10)}
	return string(s)
}
