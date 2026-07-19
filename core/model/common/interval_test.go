package common

import "testing"

func TestParseInterval(t *testing.T) {
	tests := []struct {
		in   string
		want Interval
	}{
		{"1s", Interval1s},
		{"1m", Interval1m},
		{"1", Interval1m},
		{"5m", Interval5m},
		{"5", Interval5m},
		{"1h", Interval1h},
		{"60", Interval1h},
		{"1d", Interval1d},
		{"D", Interval1d},
		{"1w", Interval1w},
		{"W", Interval1w},
		{"1M", Interval1M},
		{"M", Interval1M},
	}
	for _, tt := range tests {
		got, err := ParseInterval(tt.in)
		if err != nil {
			t.Fatalf("ParseInterval(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("ParseInterval(%q)=%v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestIntervalBybitTopic(t *testing.T) {
	tests := []struct {
		iv   Interval
		want string
		ok   bool
	}{
		{Interval1m, "1", true},
		{Interval5m, "5", true},
		{Interval1h, "60", true},
		{Interval1d, "D", true},
		{Interval1s, "", false},
		{Interval8h, "", false},
	}
	for _, tt := range tests {
		got, ok := tt.iv.BybitTopic()
		if ok != tt.ok || got != tt.want {
			t.Errorf("%v.BybitTopic()=(%q,%v), want (%q,%v)", tt.iv, got, ok, tt.want, tt.ok)
		}
	}
}
