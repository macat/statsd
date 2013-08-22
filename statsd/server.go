package main

import (
	"bytes"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TODO: clean shutdown (save and restore the live log)

type Server interface {
	Serve() error
	Inject(*Metric) error
	LiveLog(name string, chs []string) ([][]float64, int64, error)
	Log(name string, chs []string, from, length, gran int64) ([][]float64, error)
	LiveWatch(name string, chs []string) (*Watcher, error)
	Watch(name string, chs []string, offs, gran int64) (*Watcher, error)
}

type Metric struct {
	Name       string
	Type       MetricType
	Value      float64
	SampleRate float64
}

type Error string

func (err Error) Error() string {
	return string(err)
}

const (
	ErrNoName          = Error("Name missing")
	ErrNoType          = Error("Type missing")
	ErrNoValue         = Error("Value missing")
	ErrNameInvalid     = Error("Invalid characters in name")
	ErrTypeInvalid     = Error("Invalid type")
	ErrValueInvalid    = Error("Invalid value")
	ErrSamplingInvalid = Error("Invalid sample rate")
	ErrNegativeMeter   = Error("Negative meter value")
	ErrNoData          = Error("No data")
	ErrChannelInvalid  = Error("No such channel")
	ErrMixingTypes     = Error("Cannot mix different metric types")
	ErrInvalid         = Error("Invalid paramter")
	ErrNoChannels      = Error("No channels specified")
)

const MsgMaxSize = 2048
const LiveLogSize = 600

type server struct {
	sync.Mutex
	addr     *net.UDPAddr
	ds       Datastore
	metrics  [NMetricTypes]map[string]*metricEntry
	nEntries int
	lastTick int64
	notify   chan int
}

type metricEntry struct {
	metric
	sync.Mutex
	recvdInput     bool
	recvdInputTick bool
	idleTicks      int
	liveLog        []*[LiveLogSize]float64
	livePtr        int64
	lastTick       int64
	watchers       []*Watcher
}

type Watcher struct {
	Ts   int64
	C    <-chan []float64
	me   *metricEntry
	buff [][]float64
	in   chan []float64
	out  chan []float64
	chs  []int
	aggr aggregator
	gran int64
	offs int64
}

func NewServer(addr *net.UDPAddr, ds Datastore) Server {
	srv := &server{
		addr: addr,
		ds:   ds,
	}
	for i := range srv.metrics {
		srv.metrics[i] = make(map[string]*metricEntry)
	}
	return srv
}

func (srv *server) Serve() error {
	conn, err := net.ListenUDP("udp", srv.addr)
	if err != nil {
		return err
	}

	err = srv.ds.Init()
	if err != nil {
		return err
	}

	srv.lastTick = time.Now().Unix()
	go srv.tick()

	for {
		buff := make([]byte, MsgMaxSize)
		n, err := conn.Read(buff)
		if err != nil {
			log.Println(err)
			continue
		}
		go srv.processMsg(buff[0:n])
	}
}

func (srv *server) processMsg(msg []byte) {
	metrics := bytes.Split(msg, []byte{'\n'})
	for _, m := range metrics {
		if len(m) == 0 { // Silently ignore blank lines
			continue
		}
		metric, err := ParseMetric(m)
		if err != nil {
			log.Println(err)
			continue
		}
		err = srv.Inject(metric)
		if err != nil {
			log.Println(err)
		}
	}
}

func ParseMetric(m []byte) (*Metric, error) {
	// See https://github.com/b/statsd_spec

	n := bytes.IndexByte(m, ':')
	if n <= 0 { // -1 or 0
		return nil, ErrNoName
	}
	name := m[0:n]

	if n == len(m)-1 {
		return nil, ErrNoValue
	}

	if i := bytes.IndexAny(name, "/:\x00"); i != -1 {
		return nil, ErrNameInvalid
	}

	fields := bytes.Split(m[n+1:], []byte{'|'})
	if len(fields) < 2 || len(fields[1]) == 0 {
		return nil, ErrNoType
	}

	value, err := strconv.ParseFloat(string(fields[0]), 64)
	if err != nil {
		return nil, ErrValueInvalid
	}

	typ, sr := Counter, 1.0

	switch string(fields[1]) {
	case "m":
		typ = Meter
		if value < 0 {
			return nil, ErrNegativeMeter
		}

	case "c":
		typ = Counter

	case "ms":
		typ = Timer

	case "g":
		typ = Gauge

	case "a":
		typ = Avg

	default:
		return nil, ErrTypeInvalid
	}

	if len(fields) >= 3 {
		if len(fields[2]) == 0 || fields[2][0] != '@' {
			return nil, ErrSamplingInvalid
		}

		s, err := strconv.ParseFloat(string(fields[2][1:]), 64)
		if err != nil || s <= 0 {
			return nil, ErrSamplingInvalid
		}

		sr = s
	}

	return &Metric{string(name), typ, value, sr}, nil
}

func (srv *server) Inject(metric *Metric) error {
	if metric.Type < 0 || metric.Type >= NMetricTypes {
		return ErrTypeInvalid
	}
	if metric.SampleRate <= 0 {
		return ErrSamplingInvalid
	}

	if i := strings.IndexAny(metric.Name, "/:\x00"); i != -1 {
		return ErrNameInvalid
	}

	me := srv.getMetricEntry(metric.Type, metric.Name)
	defer me.Unlock()

	me.recvdInput = true
	me.recvdInputTick = true
	return me.inject(metric)
}

func (srv *server) getMetricEntry(typ MetricType, name string) *metricEntry {
	srv.Lock()
	defer srv.Unlock()

	me := srv.metrics[typ][name]
	if me == nil {
		chs := metricTypes[typ].channels

		me = &metricEntry{
			metric:   metricTypes[typ].create(),
			liveLog:  make([]*[LiveLogSize]float64, len(chs)),
			lastTick: srv.lastTick,
		}

		for i, n := range chs {
			def := srv.getChannelDefault(typ, name, n, srv.lastTick)
			live := new([LiveLogSize]float64)
			for i := range live {
				live[i] = def
			}
			me.liveLog[i] = live
		}

		srv.metrics[typ][name] = me
		srv.nEntries++
	}

	me.Lock()
	return me
}

func (srv *server) getChannelDefault(typ MetricType, name, ch string, ts int64) float64 {
	def := metricTypes[typ].defaults[ch]
	if metricTypes[typ].persist[ch] {
		rec, err := srv.ds.LatestBefore(name+":"+ch, ts)
		if err == nil {
			def = rec.Value
		} else if err != ErrNoData {
			log.Println(err)
		}
	}
	return def
}

func (srv *server) tick() {
	tickCh := time.Tick(time.Second)
	for {
		select {
		case t := <-tickCh:
			if ts := t.Unix(); ts%60 != 0 {
				srv.tickMetrics(ts)
			} else {
				srv.flushMetrics(ts)
			}
		}
	}
}

func (srv *server) getLastTick() int64 {
	srv.Lock()
	defer srv.Unlock()
	return srv.lastTick
}

func (srv *server) tickMetrics(ts int64) {
	srv.Lock()
	defer srv.Unlock()

	srv.notify = make(chan int, srv.nEntries)
	srv.lastTick = ts

	for _, metrics := range srv.metrics {
		for _, me := range metrics {
			go srv.tickMetric(ts, me)
		}
	}

	for i := 0; i < srv.nEntries; i++ {
		<-srv.notify
	}
}

func (srv *server) flushMetrics(ts int64) {
	srv.Lock()
	defer srv.Unlock()

	srv.notify = make(chan int, srv.nEntries)
	srv.lastTick = ts

	for typ, metrics := range srv.metrics {
		for name, me := range metrics {
			srv.flushOrDelete(ts, me, typ, name)
		}
	}

	for i := 0; i < srv.nEntries; i++ {
		<-srv.notify
	}
}

func (srv *server) tickMetric(ts int64, me *metricEntry) {
	me.Lock()
	defer me.Unlock()

	me.updateIdle()
	me.updateLiveLog(ts)
	srv.notify <- 1
}

func (srv *server) flushOrDelete(ts int64, me *metricEntry, typ int, n string) {
	me.Lock()
	defer me.Unlock()

	me.updateIdle()

	if me.recvdInput {
		me.recvdInput = false
		go srv.flushMetric(ts, typ, n, me)
	} else if me.idleTicks > LiveLogSize && len(me.watchers) == 0 {
		srv.nEntries--
		delete(srv.metrics[typ], n)
	} else {
		srv.notify <- 1
	}
}

func (me *metricEntry) updateIdle() {
	if me.recvdInputTick {
		me.idleTicks = 0
		me.recvdInputTick = false
	} else {
		me.idleTicks++
	}
}

func (me *metricEntry) updateLiveLog(ts int64) {
	data := me.tick()
	for ch, live := range me.liveLog {
		live[me.livePtr] = data[ch]
	}
	me.livePtr = (me.livePtr + 1) % LiveLogSize
	me.lastTick = ts

	for _, w := range me.watchers {
		if w.aggr != nil {
			continue
		}
		wdata := make([]float64, len(w.chs))
		for i, j := range w.chs {
			wdata[i] = data[j]
		}
		w.in <- wdata
	}
}

func (srv *server) flushMetric(ts int64, typ int, name string, me *metricEntry) {
	me.Lock()
	defer me.Unlock()

	me.updateLiveLog(ts)

	data := me.flush()
	for i, n := range metricTypes[typ].channels {
		err := srv.ds.Insert(name+":"+n, Record{Ts: ts, Value: data[i]})
		if err != nil {
			log.Println(err)
		}
	}

	for _, w := range me.watchers {
		if w.aggr == nil {
			continue
		}
		wdata := make([]float64, len(w.chs))
		for i, j := range w.chs {
			wdata[i] = data[j]
		}
		w.aggr.put(wdata)
		if (me.lastTick-w.offs)%int64(60*w.gran) == 0 {
			w.in <- w.aggr.get()
		}
	}

	srv.notify <- 1
}

func (srv *server) LiveLog(name string, chs []string) ([][]float64, int64, error) {
	typ, err := metricTypeByChannels(chs)
	if err != nil {
		return nil, 0, err
	}

	me := srv.getMetricEntry(typ, name)
	defer me.Unlock()

	logs, ptr := make([]*[LiveLogSize]float64, len(chs)), me.livePtr
	for i, n := range chs {
		logs[i] = me.liveLog[getChannelIndex(typ, n)]
	}

	result, ts := make([][]float64, LiveLogSize), me.lastTick-LiveLogSize
	for i := ptr; i < LiveLogSize; i++ {
		row := make([]float64, len(chs))
		for j, log := range logs {
			row[j] = log[i]
		}
		result[i-ptr] = row
	}
	for i := int64(0); i < ptr; i++ {
		row := make([]float64, len(chs))
		for j, log := range logs {
			row[j] = log[i]
		}
		result[i+LiveLogSize-ptr] = row
	}

	return result, ts, nil
}

func (srv *server) Log(name string, chs []string, from, length, gran int64) ([][]float64, error) {
	if from%60 != 0 || gran < 1 || length < 0 {
		return nil, ErrInvalid
	}
	gran60 := 60 * gran

	typ, err := metricTypeByChannels(chs)
	if err != nil {
		return nil, err
	}

	maxLength := (srv.getLastTick() - from) / gran60
	if length > maxLength {
		length = maxLength
	}

	if length <= 0 {
		return [][]float64{}, nil
	}

	aggr := metricTypes[typ].aggregator(chs)
	input, err := srv.initAggregator(aggr, name, typ, from, from+gran60*length)
	if err != nil {
		return nil, err
	}

	output := make([][]float64, length)
	for i, ts := int64(0), from+60; i < length; i++ {
		input = feedAggregator(aggr, input, ts, gran)
		ts += gran60
		output[i] = aggr.get()
	}

	return output, nil
}

func (srv *server) initAggregator(aggr aggregator, name string, typ MetricType, from, until int64) ([][]Record, error) {
	inChs := aggr.channels()
	input, tmp := make([][]Record, len(inChs)), make([]float64, len(inChs))
	for i, n := range inChs {
		in, err := srv.ds.Query(name+":"+n, from, until)
		if err != nil {
			return nil, err
		}
		input[i] = in
		tmp[i] = srv.getChannelDefault(typ, name, n, from)
	}
	aggr.init(tmp)
	return input, nil
}

func feedAggregator(aggr aggregator, in [][]Record, ts, gran int64) [][]Record {
	tmp := make([]float64, len(in))
	for j := int64(0); j < gran; j++ {
		missing := false
		for k := range tmp {
			for len(in[k]) > 0 && in[k][0].Ts < ts {
				in[k] = in[k][1:]
			}
			if len(in[k]) > 0 && in[k][0].Ts == ts {
				tmp[k] = in[k][0].Value
			} else {
				missing = true
			}
		}
		if !missing {
			aggr.put(tmp)
		}
		ts += 60
	}
	return in
}

func (srv *server) LiveWatch(name string, chs []string) (*Watcher, error) {
	typ, err := metricTypeByChannels(chs)
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		in:  make(chan []float64),
		out: make(chan []float64),
		chs: make([]int, len(chs)),
	}
	w.C = w.out

	for i, n := range chs {
		w.chs[i] = getChannelIndex(typ, n)
	}

	me := srv.getMetricEntry(typ, name)
	defer me.Unlock()
	w.me = me
	w.Ts = me.lastTick
	me.watchers = append(me.watchers, w)

	go w.run()
	return w, nil
}

func (srv *server) Watch(name string, chs []string, offs, gran int64) (*Watcher, error) {
	if offs%60 != 0 || gran < 1 {
		return nil, ErrInvalid
	}
	gran60 := int64(60 * gran)

	typ, err := metricTypeByChannels(chs)
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		in:   make(chan []float64),
		out:  make(chan []float64),
		chs:  make([]int, len(chs)),
		aggr: metricTypes[typ].aggregator(chs),
		gran: gran,
		offs: offs,
	}
	w.C = w.out

	for i, n := range chs {
		w.chs[i] = getChannelIndex(typ, n)
	}

	me := srv.getMetricEntry(typ, name)
	defer me.Unlock()
	w.me = me
	w.Ts = me.lastTick - ((me.lastTick-offs)%gran60+gran60)%gran60

	input, err := srv.initAggregator(w.aggr, name, typ, w.Ts, w.Ts+gran60)
	if err != nil {
		return nil, err
	}
	feedAggregator(w.aggr, input, w.Ts, gran)

	me.watchers = append(me.watchers, w)

	go w.run()
	return w, nil
}

func (w *Watcher) Close() {
	w.me.Lock()
	defer w.me.Unlock()

	for i, l := 0, len(w.me.watchers); i < l; i++ {
		if w.me.watchers[i] == w {
			w.me.watchers[i] = w.me.watchers[l-1]
			w.me.watchers[l-1] = nil
			w.me.watchers = w.me.watchers[:l-1]
			if cap(w.me.watchers) > 2*len(w.me.watchers) {
				w.me.watchers = append([]*Watcher(nil), w.me.watchers...)
			}
			close(w.in)
			break
		}
	}
}

func (w *Watcher) run() {
	for w.in != nil || len(w.buff) > 0 {
		out, data := chan []float64(nil), []float64(nil)
		if len(w.buff) > 0 {
			out = w.out
			data = w.buff[0]
		}
		select {
		case out <- data:
			w.buff[0] = nil
			w.buff = w.buff[1:]
		case data, ok := <-w.in:
			if !ok {
				w.in = nil
			} else {
				w.buff = append(w.buff, data) // TODO: ez így meddig nő?
			}
		}
	}
	close(w.out)
}
