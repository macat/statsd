package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"log"
	"os"
	"sync"
)

const (
	fsDsISize = 16
	fsDsDSize = 8
)

type FsDatastore struct {
	Dir     string
	NoSync  bool
	mu      sync.Mutex
	cond    sync.Cond
	streams map[string]*fsDsStream
	queue   []*fsDsStream
	running bool
	N       uint64
	wg      sync.WaitGroup
}

type fsDsStream struct {
	sync.Mutex
	ds       *FsDatastore
	name     string
	tail     []fsDsRecord
	dat, idx *os.File
	valid    bool
	lastWr   int64
	dsize    int64
	isize    int64
}

type fsDsRecord struct {
	Ts    int64
	Value float64
}

type fsDsSnapshot struct {
	ds       *FsDatastore
	tail     []fsDsRecord
	dat, idx *os.File
	lastWr   int64
	dsize    int64
	isize    int64
}

func (ds *FsDatastore) Open() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.running {
		return Error("Datastore already running")
	}

	if fi, err := os.Stat(ds.Dir); err != nil {
		return err
	} else if !fi.IsDir() {
		return Error("Not a directory: " + ds.Dir)
	}

	ds.streams = make(map[string]*fsDsStream)
	ds.cond.L = &ds.mu
	if err := ds.loadTails(); err != nil {
		ds.streams = nil
		ds.queue = nil
		return err
	}
	ds.running = true
	go ds.write(ds.N)
	return nil
}

func (ds *FsDatastore) Close() error {
	ds.mu.Lock()
	if !ds.running {
		ds.mu.Unlock()
		return Error("Datastore not running")
	}

	ds.N++
	ds.cond.Broadcast()
	for _, st := range ds.streams {
		st.Lock()
		st.Unlock()
	}
	ds.wg.Wait()

	if err := ds.saveTails(); err != nil {
		log.Println("FsDatastore.Close:", err)
		if err := os.Remove(ds.tailFile()); err != nil {
			log.Println("FsDatastore.Close:", err)
		}
	}
	ds.running = false
	ds.streams = nil
	ds.queue = nil
	ds.mu.Unlock()
	return nil
}

func (ds *FsDatastore) Insert(name string, r Record) error {
	st := ds.getStream(name)
	if st == nil {
		return Error("Datastore not running")
	}
	st.tail = append(st.tail, fsDsRecord{Ts: r.Ts, Value: r.Value})
	st.Unlock()
	return nil
}

func (ds *FsDatastore) Query(name string, from, until int64) ([]Record, error) {
	s, err := ds.takeSnapshot(name)
	if err != nil {
		return []Record{}, err
	}
	defer s.close()

	nEntries := s.isize / fsDsISize
	if nEntries == 0 {
		return []Record{}, nil
	}

	if from < 0 {
		from -= from % 60
	} else if from%60 != 0 {
		from -= from%60 - 60
	}
	if until > 0 {
		until -= until % 60
	} else if until%60 != 0 {
		until -= until%60 + 60
	}

	n, err := s.findIdx(from)
	if err != nil {
		return nil, err
	}
	if n == -1 {
		n = 0
	}

	result := make([]Record, 0)
	var ts, pos, nts, npos int64

	if ts, pos, err = s.readIdxEntry(n); err != nil {
		return nil, err
	}

	for ; n < nEntries && ts <= until; n, ts, pos = n+1, nts, npos {
		if n != nEntries-1 {
			if nts, npos, err = s.readIdxEntry(n + 1); err != nil {
				return nil, err
			}
		} else {
			npos = s.dsize
		}

		f, u := (from-ts)/60, (until-ts)/60
		if f < 0 {
			f = 0
		}
		if maxu := (npos-pos)/fsDsDSize - 1; u > maxu {
			u = maxu
		}
		if f > u {
			continue
		}

		if _, err = s.dat.Seek(pos+f*fsDsDSize, os.SEEK_SET); err != nil {
			return nil, err
		}
		data := make([]float64, u-f+1)
		if err := binary.Read(s.dat, binary.LittleEndian, data); err != nil {
			return nil, err
		}
		for i, val := range data {
			rec := Record{Ts: ts + (f+int64(i))*60, Value: val}
			result = append(result, rec)
		}
	}

	last := s.lastWr
	for _, r := range s.tail {
		if r.Ts%60 != 0 || last >= r.Ts {
			continue
		}
		if r.Ts >= from || r.Ts <= until {
			result = append(result, Record{Ts: r.Ts, Value: r.Value})
		}
		last = r.Ts
	}

	return result, nil
}

func (ds *FsDatastore) LatestBefore(name string, ts int64) (Record, error) {
	s, err := ds.takeSnapshot(name)
	if err != nil {
		return Record{}, err
	}
	defer s.close()

	if ts > 0 {
		ts -= ts % 60
	} else if ts%60 != 0 {
		ts -= ts%60 + 60
	}

	if n := s.findTail(ts); n != -1 {
		return Record{Ts: s.tail[n].Ts, Value: s.tail[n].Value}, nil
	}

	n, err := s.findIdx(ts)
	if err != nil {
		return Record{}, err
	}
	if n == -1 {
		return Record{}, ErrNoData
	}

	t, pos, err := s.readIdxEntry(n)
	if err != nil {
		return Record{}, err
	}

	var lastPos int64
	if n == s.isize/fsDsISize-1 {
		lastPos = s.dsize - fsDsDSize
	} else {
		_, p, err := s.readIdxEntry(n + 1)
		if err != nil {
			return Record{}, err
		}
		lastPos = p - fsDsDSize
	}

	if _, err := s.dat.Seek(lastPos, os.SEEK_SET); err != nil {
		return Record{}, err
	}
	var val float64
	if err := binary.Read(s.dat, binary.LittleEndian, &val); err != nil {
		return Record{}, err
	}
	return Record{Ts: t + 60*((lastPos-pos)/fsDsDSize), Value: val}, nil
}

func (ds *FsDatastore) getStream(name string) *fsDsStream {
	ds.mu.Lock()
	if !ds.running {
		ds.mu.Unlock()
		return nil
	}
	if _, ok := ds.streams[name]; !ok {
		ds.createStream(name, nil)
	}
	st := ds.streams[name]
	st.Lock()
	ds.mu.Unlock()
	return st
}

func (ds *FsDatastore) takeSnapshot(name string) (*fsDsSnapshot, error) {
	st := ds.getStream(name)
	if st == nil {
		return nil, Error("Datastore not running")
	}
	s, err := st.takeSnapshot()
	if err != nil {
		st.Unlock()
		return nil, err
	}
	st.Unlock()
	return s, nil
}

func (ds *FsDatastore) createStream(name string, tail []fsDsRecord) {
	st := &fsDsStream{
		name: name,
		tail: tail,
		ds:   ds,
	}
	ds.streams[name] = st
	ds.queue = append(ds.queue, st)
	if len(ds.queue) == 1 {
		ds.cond.Broadcast()
	}
}

func (ds *FsDatastore) write(N uint64) {
	for n := -1; ; {
		ds.mu.Lock()
		if len(ds.queue) == 0 && ds.N == N {
			ds.cond.Wait()
		}
		if ds.N != N {
			ds.mu.Unlock()
			return
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
				x := make([]*fsDsStream, l-1, 2*(l-1))
				copy(x, ds.queue)
				ds.queue = x
			}
			st.Unlock()
			ds.mu.Unlock()
		} else {
			ds.mu.Unlock()
			if err := st.flushTail(); err != nil {
				st.valid = false
				log.Println("FsDatastore.write:", err)
			}
			if cap(st.tail) > 3*len(st.tail) {
				st.tail = make([]fsDsRecord, 0, 2*len(st.tail))
			} else {
				st.tail = st.tail[:0]
			}
			st.Unlock()
		}
	}
}

func (ds *FsDatastore) tailFile() string {
	return ds.Dir + string(os.PathSeparator) + "tail_data"
}

func (ds *FsDatastore) saveTails() error {
	f, err := os.Create(ds.tailFile())
	if err != nil {
		return err
	}
	defer f.Close()
	wr, le := bufio.NewWriter(f), binary.LittleEndian

	if err = binary.Write(wr, le, uint64(len(ds.streams))); err != nil {
		return err
	}

	var (
		n  string
		st *fsDsStream
	)
	i := 0
	for n, st = range ds.streams {
		i++
		name := []byte(n)
		if err = binary.Write(wr, le, uint64(len(name))); err != nil {
			return err
		}
		if err = binary.Write(wr, le, uint64(len(st.tail))); err != nil {
			return err
		}
		if err = binary.Write(wr, le, name); err != nil {
			return err
		}
		if err = binary.Write(wr, le, st.tail); err != nil {
			return err
		}
	}

	if !ds.NoSync {
		if err = wr.Flush(); err != nil {
			return err
		}
		if err = f.Sync(); err != nil {
			return err
		}
	}
	return nil
}

func (ds *FsDatastore) loadTails() error {
	f, err := os.Open(ds.tailFile())
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	defer f.Close()
	rd, le := bufio.NewReader(f), binary.LittleEndian

	var ntails int64
	if err = binary.Read(rd, le, &ntails); err != nil {
		return err
	}

	for i := int64(0); i < ntails; i++ {
		var lname, ltail int64
		if err = binary.Read(rd, le, &lname); err != nil {
			return err
		}
		if err = binary.Read(rd, le, &ltail); err != nil {
			return err
		}
		name := make([]byte, lname)
		if err = binary.Read(rd, le, &name); err != nil {
			return err
		}
		tail := make([]fsDsRecord, ltail)
		if err = binary.Read(rd, le, &tail); err != nil {
			return err
		}
		strName := string(name)
		ds.createStream(strName, tail)
	}
	return nil
}

func (st *fsDsStream) flushTail() error {
	if err := st.openFiles(); err != nil {
		return err
	}
	defer st.closeFiles()

	dbuff, ibuff := new(bytes.Buffer), new(bytes.Buffer)
	dsize, isize, lastWr := st.dsize, st.isize, st.lastWr

	for _, r := range st.tail {
		if r.Ts%60 != 0 {
			log.Println("fsDsStream.writeTail: Timestamp not divisible by 60")
			continue
		} else if lastWr >= r.Ts {
			log.Println("fsDsStream.writeTail: Timestamp in the past")
			continue
		}

		le := binary.LittleEndian
		binary.Write(dbuff, le, r.Value)
		dsize += fsDsDSize
		lastWr += 60

		if r.Ts > lastWr {
			binary.Write(ibuff, le, []int64{r.Ts, dsize - fsDsDSize})
			isize += fsDsISize
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

func (st *fsDsStream) path() string {
	return st.ds.Dir + string(os.PathSeparator) + st.name
}

func (st *fsDsStream) openFiles() error {
	dat, err := os.OpenFile(st.path()+".dat", os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	idx, err := os.OpenFile(st.path()+".idx", os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		dat.Close()
		return err
	}
	st.dat, st.idx = dat, idx

	if !st.valid {
		di, err := dat.Stat()
		if err != nil {
			st.closeFiles()
			return err
		}
		ii, err := idx.Stat()
		if err != nil {
			st.closeFiles()
			return err
		}
		st.dsize, st.isize = di.Size(), ii.Size()
		if st.isize%fsDsISize != 0 || st.dsize%fsDsDSize != 0 {
			st.closeFiles()
			return Error("Invalid file size: " + st.name)
		}

		if st.isize == 0 {
			st.lastWr = -1<<63 - (-1<<63)%60
		} else {
			if _, err := st.idx.Seek(st.isize-fsDsISize, os.SEEK_SET); err != nil {
				st.closeFiles()
				return err
			}
			d := []int64{0, 0}
			if err := binary.Read(st.idx, binary.LittleEndian, d); err != nil {
				st.closeFiles()
				return err
			}
			ts, pos := d[0], d[1]
			st.lastWr = ts + 60*((st.dsize-pos)/fsDsDSize-1)
		}
		st.valid = true
	}

	return nil
}

func (st *fsDsStream) closeFiles() {
	if st.dat != nil {
		if !st.ds.NoSync {
			if err := st.dat.Sync(); err != nil {
				log.Println("fsDsStream.closeFiles:", err)
			}
		}
		st.dat.Close()
		st.dat = nil
	}
	if st.idx != nil {
		if !st.ds.NoSync {
			if err := st.idx.Sync(); err != nil {
				log.Println("fsDsStream.closeFiles:", err)
			}
		}
		st.idx.Close()
		st.idx = nil
	}
}

func (st *fsDsStream) takeSnapshot() (*fsDsSnapshot, error) {
	if err := st.openFiles(); err != nil {
		return nil, err
	}
	s := &fsDsSnapshot{
		ds:     st.ds,
		tail:   append([]fsDsRecord(nil), st.tail...),
		dat:    st.dat,
		idx:    st.idx,
		lastWr: st.lastWr,
		dsize:  st.dsize,
		isize:  st.isize,
	}
	st.dat, st.dat = nil, nil
	st.ds.wg.Add(1)
	return s, nil
}

func (s *fsDsSnapshot) close() {
	s.ds.wg.Done()
	s.dat.Close()
	s.idx.Close()
	s.dat, s.idx = nil, nil
}

func (s *fsDsSnapshot) findIdx(ts int64) (int64, error) {
	if s.isize == 0 {
		return -1, nil
	}

	first, _, err := s.readIdxEntry(0)
	if err != nil {
		return 0, err
	}
	if first > ts {
		return -1, nil
	}

	i, j := int64(0), s.isize/fsDsISize-1
	for i < j {
		k := (i + j) / 2
		t, _, err := s.readIdxEntry(k)
		if err != nil {
			return 0, err
		}
		switch {
		case t == ts:
			i, j = k, k
		case t > ts:
			j = k - 1
		case t < ts:
			if i != k {
				i = k
			} else {
				// j == i+1
				x, _, err := s.readIdxEntry(j)
				if err != nil {
					return 0, err
				}
				if x > ts {
					j = i
				} else {
					i = j
				}
			}
		}
	}
	return i, nil
}

func (s *fsDsSnapshot) findTail(ts int64) int64 {
	last, k := s.lastWr, -1
	for i, r := range s.tail {
		if r.Ts%60 != 0 || last >= r.Ts {
			continue
		}
		if r.Ts <= ts {
			k = i
		} else {
			break
		}
		last = r.Ts
	}
	return int64(k)
}

func (s *fsDsSnapshot) readIdxEntry(n int64) (ts int64, pos int64, err error) {
	if _, err := s.idx.Seek(n*fsDsISize, os.SEEK_SET); err != nil {
		return 0, 0, err
	}
	d := [2]int64{}
	if err := binary.Read(s.idx, binary.LittleEndian, d[:]); err != nil {
		return 0, 0, err
	}
	if d[0]%60 != 0 || d[1]%fsDsDSize != 0 {
		return 0, 0, Error("Invalid index data")
	}
	return d[0], d[1], nil
}
