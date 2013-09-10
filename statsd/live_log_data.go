package main

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"log"
)

type LiveLogData struct {
	ts      int64
	size    uint64
	entries []*liveLogEntry
}

type liveLogEntry struct {
	typ  MetricType
	name []byte
	chs  [][]byte
	data [][]float64
}

func saveLiveLogData(srv *Server) *LiveLogData {
	lld := &LiveLogData{ts: srv.lastTick, size: LiveLogSize}
	for _, metrics := range srv.metrics {
		for _, me := range metrics {
			lld.entries = append(lld.entries, newLiveLogEntry(me))
		}
	}
	return lld
}

func newLiveLogEntry(me *metricEntry) *liveLogEntry {
	chs := metricTypes[me.typ].channels
	lle := &liveLogEntry{
		typ:  me.typ,
		name: []byte(me.name),
		chs:  make([][]byte, len(chs)),
		data: make([][]float64, len(chs)),
	}

	for i, n := range chs {
		lle.chs[i] = []byte(n)
		lle.data[i] = make([]float64, LiveLogSize)
		n := copy(lle.data[i], me.liveLog[i][me.livePtr:])
		copy(lle.data[i][n:], me.liveLog[i][:me.livePtr])
	}

	return lle
}

func (lld *LiveLogData) restore(srv *Server) {
	if srv.lastTick < lld.ts {
		log.Println("Ignoring the live log (timestamp in the future)")
		return
	}
	offs := (srv.lastTick - int64(LiveLogSize)) - (lld.ts - int64(lld.size))
	if uint64(offs) >= lld.size {
		log.Println("Ignoring the live log (too old)")
		return
	}
	if offs < 0 {
		log.Println("Ignoring the live log (not enough data)")
		return
	}

	for _, e := range lld.entries {
		nameStr := string(e.name)
		if CheckMetricName(nameStr) != nil {
			log.Println("Invalid metric name in live log:", nameStr)
			continue
		}
		if e.typ < 0 || e.typ >= NMetricTypes {
			log.Println("Invalid metric type in live log:", e.typ)
			continue
		}
		chsStr := make([]string, len(e.chs))
		for i, ch := range e.chs {
			chsStr[i] = string(ch)
		}
		if t, err := metricTypeByChannels(chsStr); err != nil || t != e.typ {
			log.Println("Invalid channel list in live log:", e.typ, nameStr)
			log.Println(chsStr)
			continue
		}
		me := srv.createMetricEntry(e.typ, nameStr)
		srv.metrics[e.typ][nameStr] = me
		me.livePtr = (int64(lld.size) - offs) % LiveLogSize
		for i, ch := range chsStr {
			j := getChannelIndex(e.typ, ch)
			copy(me.liveLog[j][0:], e.data[i][offs:])
		}
	}
}

func (lld *LiveLogData) WriteTo(fn string) error {
	f, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer f.Close()
	w, le := bufio.NewWriter(f), binary.LittleEndian

	err = binary.Write(w, le, lld.ts)
	if err != nil {
		return err
	}
	err = binary.Write(w, le, lld.size)
	if err != nil {
		return err
	}
	err = binary.Write(w, le, uint64(len(lld.entries)))
	for _, lle := range lld.entries {
		if err := lle.writeTo(w); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}

	return nil
}

func (lld *LiveLogData) ReadFrom(fn string) error {
	f, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer f.Close()
	r, le := bufio.NewReader(f), binary.LittleEndian

	var (
		size     uint64
		nentries uint64
		ts       int64
	)

	if err = binary.Read(r, le, &ts); err != nil {
		return err
	}
	if err = binary.Read(r, le, &size); err != nil {
		return err
	}
	if err = binary.Read(r, le, &nentries); err != nil {
		return err
	}
	entries := make([]*liveLogEntry, nentries)
	for i := range entries {
		entries[i] = new(liveLogEntry)
		if err = entries[i].readFrom(r, size); err != nil {
			return err
		}
	}

	lld.ts = ts
	lld.size = size
	lld.entries = entries
	return nil
}

func (lle *liveLogEntry) writeTo(w io.Writer) error {
	le := binary.LittleEndian
	if err := binary.Write(w, le, lle.typ); err != nil {
		return err
	}
	if err := binary.Write(w, le, uint64(len(lle.name))); err != nil {
		return err
	}
	if err := binary.Write(w, le, lle.name); err != nil {
		return err
	}
	if err := binary.Write(w, le, uint64(len(lle.chs))); err != nil {
		return err
	}
	for i := range lle.chs {
		if err := binary.Write(w, le, uint64(len(lle.chs[i]))); err != nil {
			return err
		}
		if err := binary.Write(w, le, lle.chs[i]); err != nil {
			return err
		}
		if err := binary.Write(w, le, lle.data[i]); err != nil {
			return err
		}
	}
	return nil
}

func (lle *liveLogEntry) readFrom(r io.Reader, size uint64) error {
	le := binary.LittleEndian
	var typ MetricType
	if err := binary.Read(r, le, &typ); err != nil {
		return err
	}
	var lname uint64
	if err := binary.Read(r, le, &lname); err != nil {
		return err
	}
	name := make([]byte, lname)
	if err := binary.Read(r, le, &name); err != nil {
		return err
	}
	var nchs uint64
	if err := binary.Read(r, le, &nchs); err != nil {
		return err
	}
	chs := make([][]byte, nchs)
	data := make([][]float64, nchs)
	for i := range chs {
		var lchname uint64
		if err := binary.Read(r, le, &lchname); err != nil {
			return err
		}
		chname := make([]byte, lchname)
		if err := binary.Read(r, le, &chname); err != nil {
			return err
		}
		chdata := make([]float64, size)
		if err := binary.Read(r, le, chdata); err != nil {
			return err
		}
		chs[i] = chname
		data[i] = chdata
	}

	lle.typ = typ
	lle.name = name
	lle.chs = chs
	lle.data = data
	return nil
}
