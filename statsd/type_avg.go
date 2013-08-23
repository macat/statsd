package main

import "math"

func init() {
	mt := metricType{
		create:   func() metric { return &avgMetric{} },
		channels: []string{"avg", "avg-cnt"},
		defaults: map[string]float64{
			"avg":     math.NaN(),
			"avg-cnt": 0,
		},
		persist: map[string]bool{
			"avg":     false,
			"avg-cnt": false,
		},
		aggregator: createAvgAggregator,
	}
	registerMetricType(Avg, mt)
}

type avgMetric struct {
	tickSum, tickCount, sum, count float64
}

func (b *avgMetric) inject(metric *Metric) {
	b.tickSum += metric.Value / metric.SampleRate
	b.tickCount += 1 / metric.SampleRate
}

func (b *avgMetric) tick() []float64 {
	sum, count := b.tickSum, b.tickCount
	b.tickSum, b.tickCount = 0, 0
	b.sum += sum
	b.count += count
	return []float64{sum / count, count}
}

func (b *avgMetric) flush() []float64 {
	sum, count := b.sum, b.count
	b.sum, b.count = 0, 0
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

func (aggr *avgAggregator) channels() []string {
	if aggr.avgOut == -1 {
		return []string{"avg-cnt"}
	} else {
		return []string{"avg", "avg-cnt"}
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
