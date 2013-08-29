package main

import (
	"log"
	"sync"
	"time"
)

// TODO: clean shutdown (save and restore the live log)

type Server interface {
	Start() error
	Inject(*Metric) error
	InjectBytes([]byte)
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
	ErrNoSampling      = Error("Sample rate missing")
	ErrNameInvalid     = Error("Invalid characters in name")
	ErrTypeInvalid     = Error("Invalid type")
	ErrValueInvalid    = Error("Invalid value")
	ErrSamplingInvalid = Error("Invalid sample rate")
	ErrChannelInvalid  = Error("No such channel")
	ErrMixingTypes     = Error("Cannot mix different metric types")
	ErrInvalid         = Error("Invalid paramter")
	ErrNoChannels      = Error("No channels specified")
	ErrNonunique       = Error("Channel names must be unique")
)

const LiveLogSize = 600

type server struct {
	sync.Mutex
	ds       Datastore
	prefix   string
	metrics  [NMetricTypes]map[string]*metricEntry
	nEntries int
	lastTick int64
	notify   chan int
}

type metricEntry struct {
	metric
	sync.Mutex
	typ            MetricType
	name           string
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
	in   chan []float64
	out  chan []float64
	chs  []int
	aggr aggregator
	gran int64
	offs int64
}

func NewServer(ds Datastore, prefix string) Server {
	srv := &server{ds: ds, prefix: prefix}
	for i := range srv.metrics {
		srv.metrics[i] = make(map[string]*metricEntry)
	}
	return srv
}

func (srv *server) Start() error {
	srv.lastTick = time.Now().Unix()
	go srv.tick()
	return nil
}

func (srv *server) InjectBytes(msg []byte) {
	for i, j := 0, -1; i <= len(msg); i++ {
		if i != len(msg) && msg[i] != '\n' || i == j+1 {
			continue
		}
		metric, err := ParseMetric(msg[j+1 : i])
		j = i
		if err != nil {
			log.Println("Server.ParseMetric:", err)
			continue
		}
		err = srv.Inject(metric)
		if err != nil {
			log.Println("Server.Inject:", err)
		}
	}
}

func (srv *server) Inject(metric *Metric) error {
	if metric.Type < 0 || metric.Type >= NMetricTypes {
		return ErrTypeInvalid
	}
	if metric.SampleRate <= 0 {
		return ErrSamplingInvalid
	}

	for _, ch := range metric.Name {
		if ch == ':' || ch == '/' || ch == 0 {
			return ErrNameInvalid
		}
	}

	me := srv.getMetricEntry(metric.Type, metric.Name)
	me.recvdInput = true
	me.recvdInputTick = true
	me.inject(metric)
	me.Unlock()
	return nil
}

func (srv *server) getMetricEntry(typ MetricType, name string) *metricEntry {
	srv.Lock()

	me := srv.metrics[typ][name]
	if me == nil {
		chs := metricTypes[typ].channels

		me = &metricEntry{
			metric:   metricTypes[typ].create(),
			typ:      typ,
			name:     name,
			liveLog:  make([]*[LiveLogSize]float64, len(chs)),
			lastTick: srv.lastTick,
		}

		initData := make([]float64, len(chs))
		for i := range chs {
			def := srv.getChannelDefault(typ, name, i, srv.lastTick)
			initData[i] = def
			live := new([LiveLogSize]float64)
			for i := range live {
				live[i] = def
			}
			me.liveLog[i] = live
		}

		me.init(initData)

		srv.metrics[typ][name] = me
		srv.nEntries++
	}

	me.Lock()
	srv.Unlock()
	return me
}

func (srv *server) getChannelDefault(typ MetricType, name string, i int, ts int64) float64 {
	mt := metricTypes[typ]
	def := mt.defaults[i]
	if mt.persist[i] {
		rec, err := srv.ds.LatestBefore(srv.prefix+name+":"+mt.channels[i], ts)
		if err == nil {
			def = rec.Value
		} else if err != ErrNoData {
			log.Println("Server.getChannelDefault:", err)
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
	lt := srv.lastTick
	srv.Unlock()
	return lt
}

func (srv *server) tickMetrics(ts int64) {
	srv.Lock()

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
	srv.Unlock()
}

func (srv *server) flushMetrics(ts int64) {
	srv.Lock()

	srv.notify = make(chan int, srv.nEntries)
	srv.lastTick = ts

	for _, metrics := range srv.metrics {
		for _, me := range metrics {
			srv.flushOrDelete(ts, me)
		}
	}

	for i := 0; i < srv.nEntries; i++ {
		<-srv.notify
	}

	srv.Unlock()
}

func (srv *server) tickMetric(ts int64, me *metricEntry) {
	me.Lock()
	me.updateIdle()
	me.updateLiveLog(ts)
	srv.notify <- 1
	me.Unlock()
}

func (srv *server) flushOrDelete(ts int64, me *metricEntry) {
	me.Lock()

	me.updateIdle()

	if me.recvdInput || len(me.watchers) != 0 {
		me.recvdInput = false
		go srv.flushMetric(ts, me)
	} else if me.idleTicks > LiveLogSize {
		srv.nEntries--
		delete(srv.metrics[me.typ], me.name)
	} else {
		srv.notify <- 1
	}

	me.Unlock()
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
	var data []float64
	data = me.tick()
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

func (srv *server) flushMetric(ts int64, me *metricEntry) {
	me.Lock()

	me.updateLiveLog(ts)

	data := me.flush()
	for i, n := range metricTypes[me.typ].channels {
		err := srv.ds.Insert(srv.prefix+me.name+":"+n, Record{ts, data[i]})
		if err != nil {
			log.Println("Server.flushMetric:", err)
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
	me.Unlock()
}

func (srv *server) LiveLog(name string, chs []string) ([][]float64, int64, error) {
	typ, err := metricTypeByChannels(chs)
	if err != nil {
		return nil, 0, err
	}

	me := srv.getMetricEntry(typ, name)

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

	me.Unlock()
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
	for i, j := range inChs {
		ch := metricTypes[typ].channels[j]
		in, err := srv.ds.Query(srv.prefix+name+":"+ch, from, until)
		if err != nil {
			return nil, err
		}
		input[i] = in
		tmp[i] = srv.getChannelDefault(typ, name, j, from)
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
	w.me = me
	w.Ts = me.lastTick
	me.watchers = append(me.watchers, w)
	me.Unlock()

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
		aggr: metricTypes[typ].aggregator(chs),
		gran: gran,
		offs: offs,
	}
	w.chs = w.aggr.channels()
	w.C = w.out

	me := srv.getMetricEntry(typ, name)
	w.me = me
	w.Ts = me.lastTick - ((me.lastTick-offs)%gran60+gran60)%gran60

	input, err := srv.initAggregator(w.aggr, name, typ, w.Ts, w.Ts+gran60)
	if err != nil {
		me.Unlock()
		return nil, err
	}
	feedAggregator(w.aggr, input, w.Ts, gran)

	me.watchers = append(me.watchers, w)
	me.Unlock()

	go w.run()
	return w, nil
}

func (w *Watcher) Close() {
	w.me.Lock()
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
	w.me.Unlock()
}

func (w *Watcher) run() {
	var buff [][]float64
	for w.in != nil || len(buff) > 0 {
		out, data := chan []float64(nil), []float64(nil)
		if len(buff) > 0 {
			out = w.out
			data = buff[0]
		}
		select {
		case out <- data:
			buff[0] = nil
			buff = buff[1:]
		case data, ok := <-w.in:
			if !ok {
				w.in = nil
			} else {
				buff = append(buff, data)
			}
		}
	}
	close(w.out)
}
