package main

type MetricType int64

const (
	Counter = MetricType(iota)
	Timer
	Gauge
	Averager
	Accumulator
	NMetricTypes = iota
)

var (
	metricTypes    [NMetricTypes]metricType
	outputChannels map[string]MetricType = make(map[string]MetricType)
)

type metric interface {
	init([]float64)
	inject(*Metric)
	tick() []float64
	flush() []float64
}

type aggregator interface {
	channels() []int
	init([]float64)
	put([]float64)
	get() []float64
}

type metricType struct {
	create     func() metric
	channels   []string
	defaults   []float64
	persist    []bool
	aggregator func([]string) aggregator
}

func registerMetricType(typ MetricType, mt metricType) {
	metricTypes[typ] = mt
	for _, ch := range mt.channels {
		outputChannels[ch] = typ
	}
}

func metricTypeByChannels(chs []string) (MetricType, error) {
	if len(chs) == 0 {
		return -1, Error("No channels specified")
	}

	typ, ok := outputChannels[chs[0]]
	if !ok {
		return -1, Error("No such channel")
	}

	names := map[string]bool{chs[0]: true}
	for _, ch := range chs[1:] {
		t, ok := outputChannels[ch]
		if !ok {
			return -1, Error("No such channel")
		}
		if t != typ {
			return -1, Error("Cannot mix different metric types")
		}
		if names[ch] {
			return -1, Error("Channel names must be unique")
		}
		names[ch] = true
	}
	return typ, nil
}

func getChannelIndex(typ MetricType, ch string) int {
	for i, n := range metricTypes[typ].channels {
		if n == ch {
			return i
		}
	}
	return -1
}
