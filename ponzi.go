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

	fmt.Printf("%+v", pd[0])

	return

	if err := termbox.Init(); err != nil {
		log.Fatalf("termbox.Init: %v", err)
	}
	defer termbox.Close()

	if err := termbox.Clear(termbox.ColorDefault, termbox.ColorDefault); err != nil {
		log.Fatalf("termbox.Clear: %v", err)
	}

loop:
	for {
		termbox.SetCell(0, 0, 'S', termbox.ColorDefault, termbox.ColorDefault)
		termbox.SetCell(1, 0, 'P', termbox.ColorDefault, termbox.ColorDefault)
		termbox.SetCell(2, 0, 'Y', termbox.ColorDefault, termbox.ColorDefault)

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

func (pd priceData) Len() int {
	return len(pd)
}

func (pd priceData) Less(i, j int) bool {
	return pd[i].date.Before(pd[j].date)
}

func (pd priceData) Swap(i, j int) {
	pd[i], pd[j] = pd[j], pd[i]
}

func getPriceData(symbol string, startDate time.Time, endDate time.Time) (priceData, error) {
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

	var pd priceData
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

	sort.Sort(pd)

	return pd, nil
}
