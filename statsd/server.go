package main

import (
	"log"
	"sync"
	"time"
)

// TODO: save and restore the live log

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
	ErrInvalid = Error("Invalid paramter")
)

const LiveLogSize = 600

type Server struct {
	Ds       Datastore
	Prefix   string
	mu       sync.Mutex
	wg       sync.WaitGroup
	metrics  [NMetricTypes]map[string]*metricEntry
	running  bool
	lastTick int64
	N        uint64
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

func (srv *Server) Start() error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.running {
		return Error("Server already running")
	}

	for i := range srv.metrics {
		srv.metrics[i] = make(map[string]*metricEntry)
	}
	srv.running = true
	srv.lastTick = time.Now().Unix()
	go srv.tick(srv.N)
	return nil
}

func (srv *Server) Stop() error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !srv.running {
		return Error("Server not running")
	}

	for i, metrics := range srv.metrics {
		for _, me := range metrics {
			me.Lock()
			for _, w := range me.watchers {
				close(w.in)
			}
			me.Unlock()
		}
		srv.metrics[i] = nil
	}
	srv.running = false
	srv.N++
	return nil
}

func (srv *Server) InjectBytes(msg []byte) {
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

func (srv *Server) Inject(metric *Metric) error {
	if metric.Type < 0 || metric.Type >= NMetricTypes {
		return ErrTypeInvalid
	}
	if metric.SampleRate <= 0 {
		return ErrSamplingInvalid
	}

	for _, ch := range metric.Name {
		if ch < 32 || ch == ':' || ch == '/' || ch == '\\' || ch == '"' {
			return ErrNameInvalid
		}
	}

	me, err := srv.getMetricEntry(metric.Type, metric.Name)
	if err != nil {
		return err
	}
	me.recvdInput = true
	me.recvdInputTick = true
	me.inject(metric)
	me.Unlock()
	return nil
}

func (srv *Server) getMetricEntry(typ MetricType, name string) (*metricEntry, error) {
	if err := CheckMetricName(name); err != nil {
		return nil, err
	}

	srv.mu.Lock()
	if !srv.running {
		srv.mu.Unlock()
		return nil, Error("Server not running")
	}

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
	}

	me.Lock()
	srv.mu.Unlock()
	return me, nil
}

func (srv *Server) getChannelDefault(typ MetricType, name string, i int, ts int64) float64 {
	mt := metricTypes[typ]
	def := mt.defaults[i]
	if mt.persist[i] {
		rec, err := srv.Ds.LatestBefore(srv.Prefix+name+":"+mt.channels[i], ts)
		if err == nil {
			def = rec.Value
		} else if err != ErrNoData {
			log.Println("Server.getChannelDefault:", err)
		}
	}
	return def
}

func (srv *Server) tick(N uint64) {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case t := <-ticker.C:
			ts := t.Unix()
			srv.mu.Lock()
			if srv.N != N {
				ticker.Stop()
				srv.mu.Unlock()
				return
			}
			for srv.lastTick < ts {
				srv.lastTick++
				if srv.lastTick%60 != 0 {
					srv.tickMetrics()
				} else {
					srv.flushMetrics()
				}
			}
			srv.mu.Unlock()
		}
	}
}

func (srv *Server) tickMetrics() {
	for _, metrics := range srv.metrics {
		srv.wg.Add(len(metrics))
		for _, me := range metrics {
			go srv.tickMetric(me)
		}
	}
	srv.wg.Wait()
}

func (srv *Server) flushMetrics() {
	for _, metrics := range srv.metrics {
		for _, me := range metrics {
			srv.flushOrDelete(me)
		}
	}
	srv.wg.Wait()
}

func (srv *Server) tickMetric(me *metricEntry) {
	me.Lock()
	me.updateIdle()
	me.updateLiveLog(srv.lastTick)
	me.Unlock()
	srv.wg.Done()
}

func (srv *Server) flushOrDelete(me *metricEntry) {
	me.Lock()

	me.updateIdle()

	if me.recvdInput || len(me.watchers) != 0 {
		srv.wg.Add(1)
		go srv.flushMetric(me)
	} else if me.idleTicks > LiveLogSize {
		delete(srv.metrics[me.typ], me.name)
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

func (srv *Server) flushMetric(me *metricEntry) {
	me.Lock()

	me.updateLiveLog(srv.lastTick)
	data := me.flush()

	if me.recvdInput {
		for i, n := range metricTypes[me.typ].channels {
			dbName := srv.Prefix + me.name + ":" + n
			rec := Record{Ts: srv.lastTick, Value: data[i]}
			err := srv.Ds.Insert(dbName, rec)
			if err != nil {
				log.Println("Server.flushMetric:", err)
			}
		}
		me.recvdInput = false
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

	me.Unlock()
	srv.wg.Done()
}

func (srv *Server) LiveLog(name string, chs []string) ([][]float64, int64, error) {
	typ, err := metricTypeByChannels(chs)
	if err != nil {
		return nil, 0, err
	}

	me, err := srv.getMetricEntry(typ, name)
	if err != nil {
		return nil, 0, err
	}

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

func (srv *Server) Log(name string, chs []string, from, length, gran int64) ([][]float64, error) {
	if from%60 != 0 || gran < 1 || length < 0 {
		return nil, ErrInvalid
	}
	gran60 := 60 * gran

	typ, err := metricTypeByChannels(chs)
	if err != nil {
		return nil, err
	}

	me, err := srv.getMetricEntry(typ, name)
	if err != nil {
		return nil, err
	}

	maxLength := (me.lastTick - from) / gran60

	if length > maxLength {
		length = maxLength
	}

	if length <= 0 {
		me.Unlock()
		return [][]float64{}, nil
	}

	aggr := metricTypes[typ].aggregator(chs)
	input, err := srv.initAggregator(aggr, name, typ, from, from+gran60*length)
	if err != nil {
		me.Unlock()
		return nil, err
	}

	output := make([][]float64, length)
	for i, ts := int64(0), from; i < length; i++ {
		feedAggregator(aggr, input, ts, gran)
		ts += gran60
		output[i] = aggr.get()
	}

	me.Unlock()
	return output, nil
}

func (srv *Server) initAggregator(aggr aggregator, name string, typ MetricType, from, until int64) ([][]Record, error) {
	inChs := aggr.channels()
	input, tmp := make([][]Record, len(inChs)), make([]float64, len(inChs))
	for i, j := range inChs {
		ch := metricTypes[typ].channels[j]
		in, err := srv.Ds.Query(srv.Prefix+name+":"+ch, from+60, until)
		if err != nil {
			return nil, err
		}
		input[i] = in
		tmp[i] = srv.getChannelDefault(typ, name, j, from)
	}
	aggr.init(tmp)
	return input, nil
}

func feedAggregator(aggr aggregator, in [][]Record, ts, gran int64) {
	tmp := make([]float64, len(in))
	for j := int64(0); j < gran; j++ {
		ts += 60
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
	}
}

func (srv *Server) LiveWatch(name string, chs []string) (*Watcher, error) {
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

	me, err := srv.getMetricEntry(typ, name)
	if err != nil {
		return nil, err
	}
	w.me = me
	w.Ts = me.lastTick
	me.watchers = append(me.watchers, w)
	go w.run()

	me.Unlock()
	return w, nil
}

func (srv *Server) Watch(name string, chs []string, offs, gran int64) (*Watcher, error) {
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

	me, err := srv.getMetricEntry(typ, name)
	if err != nil {
		return nil, err
	}
	w.me = me
	w.Ts = me.lastTick - ((me.lastTick-offs)%gran60+gran60)%gran60

	input, err := srv.initAggregator(w.aggr, name, typ, w.Ts, w.Ts+gran60)
	if err != nil {
		me.Unlock()
		return nil, err
	}
	feedAggregator(w.aggr, input, w.Ts, gran)

	me.watchers = append(me.watchers, w)
	go w.run()

	me.Unlock()
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
			out, data = w.out, buff[0]
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
