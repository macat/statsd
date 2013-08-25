package main

import "math"

func init() {
	mt := metricType{
		create:   func() metric { return &avgMetric{} },
		channels: []string{"avg", "avg-cnt"},
		defaults: []float64{math.NaN(), 0},
		persist:  []bool{false, false},
		aggregator: createAvgAggregator,
	}
	registerMetricType(Avg, mt)
}

type avgMetric struct {
	tickSum, tickCount, sum, count float64
}

func (m *avgMetric) init([]float64) {
}

func (m *avgMetric) inject(metric *Metric) {
	m.tickSum += metric.Value / metric.SampleRate
	m.tickCount += 1 / metric.SampleRate
}

func (m *avgMetric) tick() []float64 {
	sum, count := m.tickSum, m.tickCount
	m.tickSum, m.tickCount = 0, 0
	m.sum += sum
	m.count += count
	return []float64{sum / count, count}
}

func (m *avgMetric) flush() []float64 {
	sum, count := m.sum, m.count
	m.sum, m.count = 0, 0
	return []float64{sum / count, count}
}

type avgAggregator struct {
	avgOut, cntOut int
	sum, cnt       float64
}

func createAvgAggregator(chs []string) aggregator {
	aggr := &avgAggregator{avgOut: -1, cntOut: -1}
	for i, ch := range chs {
		if ch == "avg" {
			aggr.avgOut = i
		} else {
			// "avg-cnt"
			aggr.cntOut = i
		}
	}
	return aggr
}

func (aggr *avgAggregator) channels() []int {
	if aggr.avgOut == -1 {
		return []int{1}
	} else {
		return []int{0, 1}
	}
}

func (aggr *avgAggregator) init([]float64) {
}

func (aggr *avgAggregator) put(data []float64) {
	if aggr.avgOut != -1 {
		aggr.sum += data[0] * data[1]
		aggr.cnt += data[1]
	} else {
		aggr.cnt += data[0]
	}
}

func (aggr *avgAggregator) get() []float64 {
	avg, cnt := aggr.sum/aggr.cnt, aggr.cnt
	aggr.sum, aggr.cnt = 0, 0
	switch aggr.avgOut {
	case -1:
		return []float64{cnt}
	case 0:
		if aggr.cntOut == -1 {
			return []float64{avg}
		} else {
			return []float64{avg, cnt}
		}
	default:
		return []float64{cnt, avg}
	}
}
