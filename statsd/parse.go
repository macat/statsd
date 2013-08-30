package main

import "strconv"

const (
	ErrNoName          = Error("Name missing")
	ErrNoType          = Error("Type missing")
	ErrNoValue         = Error("Value missing")
	ErrNoSampling      = Error("Sample rate missing")
	ErrNameInvalid     = Error("Invalid characters in name")
	ErrTypeInvalid     = Error("Invalid type")
	ErrValueInvalid    = Error("Invalid value")
	ErrSamplingInvalid = Error("Invalid sample rate")
)

func ParseMetric(m []byte) (*Metric, error) {
	var n int

	if len(m) == 0 {
		return nil, ErrNoName
	}
	n = -1
	for i, ch := range m {
		if ch == ':' {
			n = i
			break
		} else if ch == '/' || ch == 0 {
			return nil, ErrNameInvalid
		}
	}
	if n == 0 {
		return nil, ErrNoName
	} else if n == -1 || n == len(m)-1 {
		return nil, ErrNoValue
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
		return nil, ErrNoValue
	} else if n == -1 || n == len(m)-1 {
		return nil, ErrNoType
	}
	value, err := strconv.ParseFloat(string(m[:n]), 64)
	if err != nil {
		return nil, ErrValueInvalid
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
		return nil, ErrTypeInvalid
	}

	sr := 1.0
	if n != len(m) {
		if n == len(m)-1 {
			return nil, ErrNoSampling
		}
		if m[n+1] != '@' {
			return nil, ErrSamplingInvalid
		}
		s, err := strconv.ParseFloat(string(m[n+2:]), 64)
		if err != nil || s <= 0 {
			return nil, ErrSamplingInvalid
		}

		sr = s
	}

	return &Metric{string(name), typ, value, sr}, nil
}
