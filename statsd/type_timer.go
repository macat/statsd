package main

import "math"

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
		aggregator: nil, // TODO
	}
	registerMetricType(Timer, mt)
}

type timerMetric struct {
	tickData, data []float64
	tickSum, sum   float64
}

func (m *timerMetric) inject(metric *Metric) error {
	m.tickData = append(m.tickData, metric.Value)
	m.tickSum += 1 / metric.SampleRate
	return nil
}

func (m *timerMetric) tick() []float64 {
	stats := m.stats(m.tickData, m.tickSum)
	m.data = append(m.data, m.tickData...)
	m.sum += m.tickSum
	m.tickData, m.tickSum = make([]float64, 0, 2*len(m.tickData)), 0
	return stats
}

func (m *timerMetric) flush() []float64 {
	stats := m.stats(m.data, m.sum)
	m.data, m.sum = make([]float64, 0, 2*len(m.data)), 0
	return stats
}

func (m *timerMetric) stats(data []float64, sum float64) []float64 { // TODO
	return []float64{
		math.NaN(),
		math.NaN(),
		math.NaN(),
		math.NaN(),
		0,
	}
}
