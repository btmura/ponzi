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
)

// tradingSessionSource is a source of trading sessions.
type tradingSessionSource string

// List of possible dataSources.
const (
	google tradingSessionSource = "google"
	yahoo  tradingSessionSource = "yahoo"
)

// tradingSession contains stats from a single trading session.
type tradingSession struct {
	date   time.Time
	open   float64
	high   float64
	low    float64
	close  float64
	volume int64
}

func getTradingSessions(symbol string, startDate, endDate time.Time) ([]tradingSession, error) {
	tradingFuncs := map[tradingSessionSource]func(symbol string, startDate, endDate time.Time) ([]tradingSession, error){
		google: getTradingSessionsFromGoogle,
		yahoo:  getTradingSessionsFromYahoo,
	}
	for ds, tf := range tradingFuncs {
		th, err := tf(symbol, startDate, endDate)
		if err != nil {
			log.Printf("tradingFunc %s: %v", ds, err)
			continue
		}
		return th, nil
	}
	return nil, fmt.Errorf("all %d tradingFuncs failed", len(tradingFuncs))
}

func getTradingSessionsFromGoogle(symbol string, startDate, endDate time.Time) ([]tradingSession, error) {
	formatTime := func(date time.Time) string {
		return date.Format("Jan 02, 2006")
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
	log.Printf("GET %s", u)

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tss []tradingSession
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

			tss = append(tss, tradingSession{
				date:   date,
				open:   open,
				high:   high,
				low:    low,
				close:  close,
				volume: volume,
			})
		}
	}

	// Most recent trading sessions at the front.
	sort.Reverse(sortableTradingSessions(tss))

	return tss, nil
}

func getTradingSessionsFromYahoo(symbol string, startDate, endDate time.Time) ([]tradingSession, error) {
	v := url.Values{}
	v.Set("s", symbol)
	v.Set("a", strconv.Itoa(int(startDate.Month())-1))
	v.Set("b", strconv.Itoa(startDate.Day()))
	v.Set("c", strconv.Itoa(startDate.Year()))
	v.Set("d", strconv.Itoa(int(endDate.Month())-1))
	v.Set("e", strconv.Itoa(endDate.Day()))
	v.Set("f", strconv.Itoa(endDate.Year()))
	v.Set("g", "d")
	v.Set("ignore", ".csv")

	u, err := url.Parse("http://ichart.yahoo.com/table.csv")
	if err != nil {
		return nil, err
	}
	u.RawQuery = v.Encode()
	log.Printf("GET %s", u)

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tss []tradingSession
	r := csv.NewReader(resp.Body)
	for i := 0; ; i++ {
		record, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// format: Date, Open, High, Low, Close, Volume, Adj. Close
		if len(record) != 7 {
			return nil, fmt.Errorf("record length should be 7, got %d", len(record))
		}

		// skip header row
		if i != 0 {
			parseRecordTime := func(i int) (time.Time, error) {
				return time.Parse("2006-01-02", record[i])
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

			// Ignore adjusted close value to keep Google and Yahoo APIs the same.

			tss = append(tss, tradingSession{
				date:   date,
				open:   open,
				high:   high,
				low:    low,
				close:  close,
				volume: volume,
			})
		}
	}

	// Most recent trading sessions at the front.
	sort.Reverse(sortableTradingSessions(tss))

	return tss, nil
}

// sortableTradingSessions is a sortable tradingSession slice.
type sortableTradingSessions []tradingSession

// Len implements sort.Interface.
func (sts sortableTradingSessions) Len() int {
	return len(sts)
}

// Less implements sort.Interface.
func (sts sortableTradingSessions) Less(i, j int) bool {
	return sts[i].date.Before(sts[j].date)
}

// Swap implements sort.Interface.
func (sts sortableTradingSessions) Swap(i, j int) {
	sts[i], sts[j] = sts[j], sts[i]
}
