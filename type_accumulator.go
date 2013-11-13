package main

func init() {
	mt := metricType{
		create:     func() metric { return &accMetric{} },
		channels:   []string{"acc"},
		defaults:   []float64{0},
		persist:    []bool{true},
		aggregator: func([]string) aggregator { return &accAggregator{} },
	}
	registerMetricType(Accumulator, mt)
}

type accMetric struct {
	value float64
}

func (m *accMetric) init(data []float64) {
	m.value = data[0]
}

func (m *accMetric) inject(metric *Metric) {
	m.value += metric.Value / metric.SampleRate
}

func (m *accMetric) tick() []float64 {
	return []float64{m.value}
}

func (m *accMetric) flush() []float64 {
	return []float64{m.value}
}

type accAggregator struct {
	value float64
}

func (aggr *accAggregator) channels() []int {
	return []int{0}
}

func (aggr *accAggregator) init(data []float64) {
	aggr.value = data[0]
}

func (aggr *accAggregator) put(data []float64) {
	aggr.value = data[0]
}

func (aggr *accAggregator) get() []float64 {
	return []float64{aggr.value}
}
