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
		aggregator: nil, // TODO
	}
	registerMetricType(Avg, mt)
}

type avgMetric struct {
	tickSum, tickCount, sum, count float64
}

func (b *avgMetric) inject(metric *Metric) error {
	b.tickSum += metric.Value / metric.SampleRate
	b.tickCount += 1 / metric.SampleRate
	return nil
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
