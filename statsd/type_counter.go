package main

func init() {
	mt := metricType{
		create:     func() metric { return &counterMetric{} },
		channels:   []string{"counter"},
		defaults:   map[string]float64{"counter": 0},
		persist:    map[string]bool{"counter": false},
		aggregator: func([]string) aggregator { return &counterAggregator{} },
	}
	registerMetricType(Counter, mt)
}

type counterMetric struct {
	tickSum, sum float64
}

func (m *counterMetric) inject(metric *Metric) {
	m.tickSum += metric.Value / metric.SampleRate
}

func (m *counterMetric) tick() []float64 {
	sum := m.tickSum
	m.sum += sum
	m.tickSum = 0
	return []float64{sum}
}

func (m *counterMetric) flush() []float64 {
	sum := m.sum
	m.sum = 0
	return []float64{sum}
}

type counterAggregator struct {
	sum float64
}

func (aggr *counterAggregator) channels() []string {
	return []string{"counter"}
}

func (aggr *counterAggregator) init([]float64) {
}

func (aggr *counterAggregator) put(data []float64) {
	aggr.sum += data[0]
}

func (aggr *counterAggregator) get() []float64 {
	sum := aggr.sum
	aggr.sum = 0
	return []float64{sum}
}
