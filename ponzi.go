package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/nsf/termbox-go"
)

func main() {
	pd, err := getPriceData("SPY", time.Now().Add(time.Hour*24*5), time.Now())
	if err != nil {
		log.Fatalf("getPriceData: %v", err)
	}

	if err := termbox.Init(); err != nil {
		log.Fatalf("termbox.Init: %v", err)
	}
	defer termbox.Close()

	if err := termbox.Clear(termbox.ColorDefault, termbox.ColorDefault); err != nil {
		log.Fatalf("termbox.Clear: %v", err)
	}

	printTerm := func(x, y int, format string, a ...interface{}) {
		for _, rune := range fmt.Sprintf(format, a...) {
			termbox.SetCell(x, y, rune, termbox.ColorDefault, termbox.ColorDefault)
			x++
		}
	}

	sort.Reverse(priceData(pd))

loop:
	for {
		if len(pd) > 0 {
			printTerm(0, 0, "SPY %s %.2f", pd[0].date.Format("1/2/06"), pd[0].close)
		}

		if err := termbox.Flush(); err != nil {
			log.Fatalf("termbox.Flush: %v", err)
		}

		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Key == termbox.KeyCtrlC {
				break loop
			}
		}
	}
}

type price struct {
	date   time.Time
	open   float64
	high   float64
	low    float64
	close  float64
	volume int64
}

type priceData []price

// Len implements sort.Interface.
func (pd priceData) Len() int {
	return len(pd)
}

// Less implements sort.Interface.
func (pd priceData) Less(i, j int) bool {
	return pd[i].date.Before(pd[j].date)
}

// Swap implements sort.Interface.
func (pd priceData) Swap(i, j int) {
	pd[i], pd[j] = pd[j], pd[i]
}

func getPriceData(symbol string, startDate time.Time, endDate time.Time) ([]price, error) {
	formatTime := func(date time.Time) string {
		return date.Format("Jan/02/06")
	}

	v := url.Values{}
	v.Set("q", symbol)
	v.Set("startdate", formatTime(startDate))
	v.Set("enddate", formatTime(endDate))
	v.Set("output", "csv")

	u, err := url.Parse("http://www.google.com/finance/historical")
	if err != nil {
		return nil, err
	}
	u.RawQuery = v.Encode()
	log.Printf("url: %s", u)

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pd []price
	r := csv.NewReader(resp.Body)
	for i := 0; ; i++ {
		record, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// format: Date, Open, High, Low, Close, Volume
		if len(record) != 6 {
			return nil, fmt.Errorf("record length should be 6, got %d", len(record))
		}

		// skip header row
		if i != 0 {
			parseRecordTime := func(i int) (time.Time, error) {
				return time.Parse("2-Jan-06", record[i])
			}

			parseRecordFloat := func(i int) (float64, error) {
				return strconv.ParseFloat(record[i], 64)
			}

			parseRecordInt := func(i int) (int64, error) {
				return strconv.ParseInt(record[i], 10, 64)
			}

			date, err := parseRecordTime(0)
			if err != nil {
				return nil, err
			}

			open, err := parseRecordFloat(1)
			if err != nil {
				return nil, err
			}

			high, err := parseRecordFloat(2)
			if err != nil {
				return nil, err
			}

			low, err := parseRecordFloat(3)
			if err != nil {
				return nil, err
			}

			close, err := parseRecordFloat(4)
			if err != nil {
				return nil, err
			}

			volume, err := parseRecordInt(5)
			if err != nil {
				return nil, err
			}

			pd = append(pd, price{
				date:   date,
				open:   open,
				high:   high,
				low:    low,
				close:  close,
				volume: volume,
			})
		}
	}

	return pd, nil
}
