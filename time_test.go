package main

import (
	"fmt"
	"testing"
	"time"
)

func TestIsMarketHours(t *testing.T) {
	parseTime := func(value string) time.Time {
		t, err := time.ParseInLocation("Mon 1/2/06 3:04 PM", value, newYorkLoc)
		if err != nil {
			panic(fmt.Sprintf("time.Parse: %v", err))
		}
		return t
	}

	for _, tt := range []struct {
		desc string
		now  time.Time
		want bool
	}{
		{
			desc: "open between hours",
			now:  parseTime("Mon 9/14/15 10:00 AM"),
			want: true,
		},
		{
			desc: "too early",
			now:  parseTime("Mon 9/14/15 9:00 AM"),
		},
		{
			desc: "too late",
			now:  parseTime("Mon 9/14/15 6:00 PM"),
		},
		{
			desc: "closed on saturday",
			now:  parseTime("Sat 9/12/15 10:00 AM"),
		},
		{
			desc: "closed on sunday",
			now:  parseTime("Sun 9/13/15 10:00 AM"),
		},
	} {
		getNow = func() time.Time {
			return tt.now
		}
		if got := isMarketHours(); got != tt.want {
			t.Errorf("[%s] %v isMarketHours() = %t, want %t", tt.desc, tt.now, got, tt.want)
		}
	}
}
