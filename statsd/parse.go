package main

import "strconv"

func ParseMetric(m []byte) (*Metric, error) {
	var n int

	if len(m) == 0 {
		return nil, Error("Metric name missing")
	}
	n = -1
	for i, ch := range m {
		if ch == ':' {
			n = i
			break
		} else if ch < 32 || ch == '/' || ch == '\\' || ch == '"' {
			return nil, Error("Invalid characters in metric name")
		}
	}
	if n == 0 {
		return nil, Error("Metric name missing")
	} else if n == -1 || n == len(m)-1 {
		return nil, Error("Metric value missing")
	}
	name := m[:n]

	n, m = -1, m[n+1:]
	for i, ch := range m {
		if ch == '|' {
			n = i
			break
		}
	}
	if n == 0 {
		return nil, Error("Metric value missing")
	} else if n == -1 || n == len(m)-1 {
		return nil, Error("Metric type missing")
	}
	value, err := strconv.ParseFloat(string(m[:n]), 64)
	if err != nil {
		return nil, Error("Metric value invalid")
	}

	n, m = -1, m[n+1:]
	for i, ch := range m {
		if ch == '|' {
			n = i
			break
		}
	}
	if n == -1 {
		n = len(m)
	}
	typ := MetricType(-1)
	if n == 1 {
		switch m[0] {
		case 'c':
			typ = Counter
		case 'g':
			typ = Gauge
		case 'a':
			typ = Averager
		}
	} else if n == 2 {
		if m[0] == 'm' && m[1] == 's' {
			typ = Timer
		} else if m[0] == 'a' && m[1] == 'c' {
			typ = Accumulator
		}
	}
	if typ == MetricType(-1) {
		return nil, Error("Metric type invalid")
	}

	sr := 1.0
	if n != len(m) {
		if n == len(m)-1 {
			return nil, Error("Sample rate missing")
		}
		if m[n+1] != '@' {
			return nil, Error("Sample rate invalid")
		}
		s, err := strconv.ParseFloat(string(m[n+2:]), 64)
		if err != nil || s <= 0 {
			return nil, Error("Sample rate invalid")
		}

		sr = s
	}

	return &Metric{string(name), typ, value, sr}, nil
}

func CheckMetricName(name string) error {
	if len(name) == 0 {
		return Error("Empty metric name")
	}
	for _, ch := range name {
		if ch < 32 || ch == '/' || ch == '\\' || ch == '"' || ch == ':' {
			return Error("Invalid characters in metric name")
		}
	}
	return nil
}
