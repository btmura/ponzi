package main

import (
	"fmt"
	"time"
)

func isMarketHours() bool {
	now := time.Now().In(newYorkLoc)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return false
	}

	opening := time.Date(now.Year(), now.Month(), now.Day(), 9, 30, 0, 0, newYorkLoc)
	if now.Before(opening) {
		return false
	}

	closing := time.Date(now.Year(), now.Month(), now.Day(), 12+4, 0, 0, 0, newYorkLoc)
	if now.After(closing) {
		return false
	}

	return true
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(fmt.Sprintf("time.LoadLocation: %v", err))
	}
	return loc
}

type sortableTimes []time.Time

// Len implements sort.Interface
func (st sortableTimes) Len() int {
	return len(st)
}

// Less implements sort.Interface
func (st sortableTimes) Less(i, j int) bool {
	return st[i].Before(st[j])
}

// Swap implements sort.Interface
func (st sortableTimes) Swap(i, j int) {
	st[i], st[j] = st[j], st[i]
}
