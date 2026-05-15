package scheduler

import (
	"testing"
	"time"
)

func TestWindowContains(t *testing.T) {
	previous := time.Local
	time.Local = time.UTC
	defer func() { time.Local = previous }()

	tests := []struct {
		name   string
		window string
		at     string
		want   bool
	}{
		{name: "same-day start inclusive", window: "09:00-17:00", at: "2026-05-15T09:00:00Z", want: true},
		{name: "same-day middle", window: "09:00-17:00", at: "2026-05-15T12:30:00Z", want: true},
		{name: "same-day end exclusive", window: "09:00-17:00", at: "2026-05-15T17:00:00Z", want: false},
		{name: "cross-midnight late", window: "22:00-06:00", at: "2026-05-15T23:00:00Z", want: true},
		{name: "cross-midnight early", window: "22:00-06:00", at: "2026-05-15T05:59:00Z", want: true},
		{name: "cross-midnight outside", window: "22:00-06:00", at: "2026-05-15T12:00:00Z", want: false},
		{name: "full-day start", window: "00:00-24:00", at: "2026-05-15T00:00:00Z", want: true},
		{name: "full-day end", window: "00:00-24:00", at: "2026-05-15T23:59:00Z", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, test.at)
			if err != nil {
				t.Fatal(err)
			}
			got, err := InWindow(test.window, at)
			if err != nil {
				t.Fatalf("in window: %v", err)
			}
			if got != test.want {
				t.Fatalf("in window = %v, want %v", got, test.want)
			}
		})
	}
}

func TestWindowUsesLocalTime(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("otis-test", -4*60*60)
	defer func() { time.Local = previous }()

	at := time.Date(2026, 5, 15, 14, 30, 0, 0, time.UTC)
	got, err := InWindow("10:00-11:00", at)
	if err != nil {
		t.Fatalf("in window: %v", err)
	}
	if !got {
		t.Fatal("expected UTC 14:30 to be inside local 10:00-11:00 window")
	}
}
