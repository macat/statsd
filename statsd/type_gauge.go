package main

func init() {
	mt := metricType{
		create:     func() metric { return &gaugeMetric{} },
		channels:   []string{"gauge"},
		defaults:   []float64{0},
		persist:    []bool{true},
		aggregator: func([]string) aggregator { return &gaugeAggregator{} },
	}
	registerMetricType(Gauge, mt)
}

type gaugeMetric struct {
	value float64
}

func (m *gaugeMetric) inject(metric *Metric) {
	m.value = metric.Value
}

func (m *gaugeMetric) tick() []float64 {
	return []float64{m.value}
}

func (m *gaugeMetric) flush() []float64 {
	return []float64{m.value}
}

type gaugeAggregator struct {
	value float64
}

func (aggr *gaugeAggregator) channels() []int {
	return []int{0}
}

func (aggr *gaugeAggregator) init(data []float64) {
	aggr.value = data[0]
}

func (aggr *gaugeAggregator) put(data []float64) {
	aggr.value = data[0]
}

func (aggr *gaugeAggregator) get() []float64 {
	return []float64{aggr.value}
}
