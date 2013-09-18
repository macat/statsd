package main

import "testing"

func TestParseMetric(t *testing.T) {
	var testCases = []struct {
		s string
		m *Metric
	}{
		{"", nil},
		{"test", nil},
		{"te/st", nil},
		{"test:", nil},
		{"test:X", nil},
		{"test:1", nil},
		{"test:1.", nil},
		{"test:1.X", nil},
		{"test:1.5", nil},
		{"test:1.5X", nil},
		{"test:1.5|", nil},
		{"test:1.5|X", nil},
		{"test:1.5|c|", nil},
		{"test:1.5|c|X", nil},
		{"test:1.5|c|@", nil},
		{"test:1.5|c|@X", nil},
		{"test:1.5|c|@0", nil},
		{"test:1.5|c|@0X", nil},
		{"test:1.5|c|@0.", nil},
		{"test:1.5|c|@0.X", nil},
		{"test:1.5|c|@0.1X", nil},
		{"te/st:1.5|c|@0.1", nil},
		{"te\\st:1.5|c|@0.1", nil},
		{"te\nst:1.5|c|@0.1", nil},
		{"te\"st:1.5|c|@0.1", nil},
		{"1.5|c|@0.1", nil},
		{":1.5|c|@0.1", nil},
		{"test:|c|@0.1", nil},
		{"test:1.5||@0.1", nil},
		{"test:1.5||", nil},
		{"test:1.5|c|@0", nil},
		{"test:1.5|c", &Metric{"test", Counter, 1.5, 1.0}},
		{"test:1.5|c|@0.1", &Metric{"test", Counter, 1.5, 0.1}},
		{"test:1.5|g", &Metric{"test", Gauge, 1.5, 1.0}},
		{"test:1.5|a", &Metric{"test", Averager, 1.5, 1.0}},
		{"test:1.5|ms", &Metric{"test", Timer, 1.5, 1.0}},
		{"test:1.5|ac", &Metric{"test", Accumulator, 1.5, 1.0}},
		{"test:1.5|x", nil},
		{"test:1.5|xy", nil},
		{"test:1.5|xyz", nil},
	}

	for _, tc := range testCases {
		m, err := ParseMetric([]byte(tc.s))
		if tc.m == nil {
			if m != nil {
				t.Error("Parsing should have failed:", tc.s)
				t.Error("Returned:", *m)
			} else if err == nil {
				t.Error("nil error:", tc.s)
			}
		} else {
			if m == nil {
				t.Error("Parsing shouldn't have failed:", tc.s)
				t.Error("Error:", err)
				t.Error("Expected:", *tc.m)
			} else if err != nil {
				t.Error("Non-nil error:", tc.s)
				t.Error("Error:", err)
			} else if *tc.m != *m {
				t.Error("Incorrect result:", tc.s)
				t.Error("Expected:", *tc.m)
				t.Error("Returned:", *m)
			}
		}
		if t.Failed() {
			return
		}
	}
}

func TestCheckMetricName(t *testing.T) {
	var testCases = []struct {
		s  string
		ok bool
	}{
		{"", false},
		{"te/st", false},
		{"te\\st", false},
		{"te\nst", false},
		{"te:st", false},
		{"te\"st", false},
		{"test", true},
	}

	for _, tc := range testCases {
		err := CheckMetricName(tc.s)
		if tc.ok {
			if err != nil {
				t.Error("Should have been accepted:", tc.s)
				t.Error("Error:", err)
			}
		} else {
			if err == nil {
				t.Error("Shouldn't have been accepted:", tc.s)
			}
		}
		if t.Failed() {
			return
		}
	}
}
