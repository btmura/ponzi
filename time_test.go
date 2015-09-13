package main

import (
	"testing"
	"time"
)

func TestIsMarketHours(t *testing.T) {
	for _, tt := range []struct {
		desc string
		now  time.Time
		want bool
	}{
		{
			desc: "open between hours",
			now:  time.Date(2015, time.September, 14, 10, 0, 0, 0, newYorkLoc),
			want: true,
		},
		{
			desc: "too early",
			now:  time.Date(2015, time.September, 14, 9, 0, 0, 0, newYorkLoc),
		},
		{
			desc: "too late",
			now:  time.Date(2015, time.September, 14, 12+6, 0, 0, 0, newYorkLoc),
		},
		{
			desc: "closed on saturday",
			now:  time.Date(2015, time.September, 12, 10, 0, 0, 0, newYorkLoc),
		},
		{
			desc: "closed on sunday",
			now:  time.Date(2015, time.September, 13, 10, 0, 0, 0, newYorkLoc),
		},
	} {
		getNow = func() time.Time {
			return tt.now
		}
		if got := isMarketHours(); got != tt.want {
			t.Errorf("[%s] isMarketHours() = %t, want %t", tt.desc, got, tt.want)
		}
	}
}
