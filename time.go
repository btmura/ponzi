package main

import "time"

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
