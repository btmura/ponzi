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

var (
	// Google is a tradingHistoryProvider that gets data from Google Finance.
	Google = tradingHistoryProvider{
		getTradingHistory: getTradingHistoryFromGoogle,
	}

	// Yahoo is a tradingHistoryProvider that gets data from Yahoo Finance.
	Yahoo = tradingHistoryProvider{
		getTradingHistory: getTradingHistoryFromYahoo,
	}
)

// tradingHistoryProvider is a struct that provides a getTradingHistory function.
type tradingHistoryProvider struct {
	getTradingHistory func(symbol string, startDate, endDate time.Time) (tradingHistory, error)
}

// tradingHistory is a sorted tradingSession slice with the most recent at the front.
type tradingHistory []tradingSession

// Len implements sort.Interface.
func (th tradingHistory) Len() int {
	return len(th)
}

// Less implements sort.Interface.
func (th tradingHistory) Less(i, j int) bool {
	return th[i].date.Before(th[j].date)
}

// Swap implements sort.Interface.
func (th tradingHistory) Swap(i, j int) {
	th[i], th[j] = th[j], th[i]
}

// tradingSession contains stats from a single trading session.
type tradingSession struct {
	date   time.Time
	open   float64
	high   float64
	low    float64
	close  float64
	volume int64
}

func getTradingHistoryFromGoogle(symbol string, startDate, endDate time.Time) (tradingHistory, error) {
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

	var th tradingHistory
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

			th = append(th, tradingSession{
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
	sort.Reverse(th)

	return th, nil
}

func getTradingHistoryFromYahoo(symbol string, startDate, endDate time.Time) (tradingHistory, error) {
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
	log.Printf("url: %s", u)

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var th tradingHistory
	r := csv.NewReader(resp.Body)
	for i := 0; ; i++ {
		record, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		log.Printf("record: %+v", record)

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

			th = append(th, tradingSession{
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
	sort.Reverse(th)

	return th, nil
}
