package main

func init() {
	mt := metricType{
		create:     func() metric { return &meterMetric{} },
		channels:   []string{"meter"},
		defaults:   map[string]float64{"meter": 0},
		persist:    map[string]bool{"meter": false},
		aggregator: func([]string) aggregator { return &meterAggregator{} },
	}
	registerMetricType(Meter, mt)
}

type meterMetric struct {
	tickSum, sum float64
}

func (m *meterMetric) inject(metric *Metric) error {
	if metric.Value < 0 {
		return ErrNegativeMeter
	}
	m.tickSum += metric.Value / metric.SampleRate
	return nil
}

func (m *meterMetric) tick() []float64 {
	sum := m.tickSum
	m.sum += sum
	m.tickSum = 0
	return []float64{sum}
}

func (m *meterMetric) flush() []float64 {
	sum := m.sum
	m.sum = 0
	return []float64{sum}
}

type meterAggregator struct {
	sum float64
}

func (aggr *meterAggregator) channels() []string {
	return []string{"meter"}
}

func (aggr *meterAggregator) init([]float64) {
}

func (aggr *meterAggregator) put(data []float64) {
	aggr.sum += data[0]
}

func (aggr *meterAggregator) get() []float64 {
	sum := aggr.sum
	aggr.sum = 0
	return []float64{sum}
}
