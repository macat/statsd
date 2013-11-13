package main

import "testing"

func TestMetricTypeByChannels(t *testing.T) {
	var testCases = []struct {
		chs []string
		typ MetricType
	}{
		{[]string{}, -1},
		{[]string{"xyz"}, -1},
		{[]string{"avg"}, Averager},
		{[]string{"avg", "xyz"}, -1},
		{[]string{"avg", "counter"}, -1},
		{[]string{"avg", "avg-cnt"}, Averager},
		{[]string{"avg", "avg-cnt", "counter"}, -1},
	}

	for _, tc := range testCases {
		mt, err := metricTypeByChannels(tc.chs)
		if tc.typ == -1 {
			if mt != -1 {
				t.Error("Should have failed:", tc.chs)
			} else if err == nil {
				t.Error("nil error:", tc.chs)
			}
		} else {
			if mt == -1 {
				t.Error("Shouldn't have failed:", tc.chs)
				t.Error("Error:", err)
				t.Error("Expected:", tc.typ)
			} else if mt != tc.typ {
				t.Error("Incorrect result:", tc.chs)
				t.Error("Expected:", tc.typ)
				t.Error("Result:", mt)
			} else if err != nil {
				t.Error("Non-nil error:", tc.chs)
				t.Error("Error:", err)
			}
		}
		if t.Failed() {
			return
		}
	}
}

func TestGetChannelIndex(t *testing.T) {
	var testCases = []struct {
		typ MetricType
		ch  string
		ok  bool
	}{
		{Counter, "counter", true},
		{Timer, "timer-min", true},
		{Gauge, "gauge", true},
		{Averager, "avg", true},
		{Accumulator, "acc", true},
		{Counter, "xyz", false},
		{Counter, "timer-min", false},
	}

	for _, tc := range testCases {
		i := getChannelIndex(tc.typ, tc.ch)
		if tc.ok {
			if i == -1 {
				t.Error("Shouldn't have failed:", tc.typ, tc.ch)
			} else if metricTypes[tc.typ].channels[i] != tc.ch {
				t.Error("Incorrect result:", tc.typ, tc.ch)
				t.Error("Result:", i)
			}
		} else {
			if i != -1 {
				t.Error("Should have failed:", tc.typ, tc.ch)
				t.Error("Result:", i)
			}
		}
		if t.Failed() {
			return
		}
	}
}
