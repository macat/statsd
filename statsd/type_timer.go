package main

import (
	"math"
	"sort"
)

func init() {
	mt := metricType{
		create: func() metric { return &timerMetric{} },
		channels: []string{
			"timer-min",
			"timer-quart1",
			"timer-median",
			"timer-quart3",
			"timer-max",
			"timer-cnt",
		},
		defaults: map[string]float64{
			"timer-min":    math.NaN(),
			"timer-quart1": math.NaN(),
			"timer-median": math.NaN(),
			"timer-quart3": math.NaN(),
			"timer-max":    math.NaN(),
			"timer-cnt":    0,
		},
		persist: map[string]bool{
			"timer-min":    false,
			"timer-quart1": false,
			"timer-median": false,
			"timer-quart3": false,
			"timer-max":    false,
			"timer-cnt":    false,
		},
		aggregator: createTimerAggregator,
	}
	registerMetricType(Timer, mt)
}

type timerMetric struct {
	tickData, data []float64
	tickCnt, cnt   []float64
}

func (m *timerMetric) inject(metric *Metric) {
	m.tickData = append(m.tickData, metric.Value)
	m.tickCnt = append(m.tickCnt, 1/metric.SampleRate)
}

func (m *timerMetric) tick() []float64 {
	stats := timerStats(m.tickData, m.tickCnt)
	m.data = append(m.data, m.tickData...)
	m.cnt = append(m.cnt, m.tickCnt...)
	m.tickData = make([]float64, 0, 2*len(m.tickData))
	m.tickCnt = make([]float64, 0, len(m.tickData))
	return stats
}

func (m *timerMetric) flush() []float64 {
	stats := timerStats(m.data, m.cnt)
	m.data = make([]float64, 0, 2*len(m.data))
	m.cnt = make([]float64, 0, len(m.data))
	return stats
}

func timerStats(data []float64, cnt []float64) []float64 {
	if nan := math.NaN(); len(data) == 0 {
		return []float64{nan, nan, nan, nan, nan, 0}
	}

	var quart1, median, quart3, n float64
	for _, v := range cnt {
		n += v
	}
	sort.Sort(&timerSorter{data, cnt})
	for i, m := 0, float64(0); i < len(data); i++ {
		if m+cnt[i] >= n*0.25 && m < n*0.25 {
			quart1 = data[i]
		}
		if m+cnt[i] >= n*0.50 && m < n*0.50 {
			median = data[i]
		}
		if m+cnt[i] >= n*0.75 && m < n*0.75 {
			quart3 = data[i]
		}
		m += cnt[i]
	}
	return []float64{data[0], quart1, median, quart3, data[len(data)-1], n}
}

type timerSorter struct {
	data, cnt []float64
}

func (s *timerSorter) Len() int {
	return len(s.data)
}

func (s *timerSorter) Less(i, j int) bool {
	return s.data[i] < s.data[j]
}

func (s *timerSorter) Swap(i, j int) {
	t1, t2 := s.data[i], s.cnt[i]
	s.data[i], s.cnt[i] = s.data[j], s.cnt[j]
	s.data[j], s.cnt[j] = t1, t2
}

type timerAggregator struct {
	chs []int
	data, cnt []float64
}

func createTimerAggregator(chs []string) aggregator {
	aggr := &timerAggregator{chs: make([]int, len(chs))}
	for i, ch := range chs {
		for j, ch2 := range metricTypes[Timer].channels {
			if ch == ch2 {
				aggr.chs[i] = j
				break
			}
		}
	}
	return aggr
}

func (aggr *timerAggregator) channels() []string {
	return []string{
		"timer-min",
		"timer-quart1",
		"timer-median",
		"timer-quart3",
		"timer-max",
		"timer-cnt",
	}
}

func (aggr *timerAggregator) init(data []float64) {
}

func (aggr *timerAggregator) put(data []float64) {
	aggr.data = append(aggr.data, data[0], data[1], data[2], data[3], data[4])
	aggr.cnt = append(aggr.cnt, data[5], data[5], data[5], data[5], data[5])
}

func (aggr *timerAggregator) get() []float64 {
	// TODO: optimize
	stats := timerStats(aggr.data, aggr.cnt)
	stats[5] /= 5
	r := make([]float64, len(aggr.chs))
	for i, j := range aggr.chs {
		r[i] = stats[j]
	}
	aggr.data = make([]float64, 0, len(aggr.data))
	aggr.cnt = make([]float64, 0, len(aggr.data))
	return r
}
