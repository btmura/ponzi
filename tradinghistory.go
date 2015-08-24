package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

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

type realTimeTradingData struct {
	timestamp     time.Time
	price         float64
	change        float64
	percentChange float64
}

func getTradingHistory(symbol string, startDate, endDate time.Time) (tradingHistory, error) {
	tradingFuncs := []func(symbol string, startDate, endDate time.Time) (tradingHistory, error){
		getTradingHistoryFromGoogle,
		getTradingHistoryFromYahoo,
	}
	for _, n := range rand.Perm(len(tradingFuncs)) {
		th, err := tradingFuncs[n](symbol, startDate, endDate)
		if err != nil {
			log.Printf("tradingFunc %d: %v", n, err)
			continue
		}
		return th, nil
	}
	return nil, fmt.Errorf("all %d tradingFuncs failed", len(tradingFuncs))
}

func getTradingHistoryFromGoogle(symbol string, startDate, endDate time.Time) (tradingHistory, error) {
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

func getRealTimeTradingData(symbol string) (realTimeTradingData, error) {
	v := url.Values{}
	v.Set("client", "ig")
	v.Set("q", symbol)

	u, err := url.Parse("http://www.google.com/finance/info")
	if err != nil {
		return realTimeTradingData{}, err
	}
	u.RawQuery = v.Encode()
	log.Printf("url: %s", u)

	resp, err := http.Get(u.String())
	if err != nil {
		return realTimeTradingData{}, err
	}
	defer resp.Body.Close()

	parsed := []struct {
		L      string // price
		C      string // change
		Cp     string // percent change
		Lt_dts string // time
	}{}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return realTimeTradingData{}, err
	}

	// Check that data has the expected "//" comment string to trim off.
	if len(data) < 3 {
		return realTimeTradingData{}, fmt.Errorf("expected data should be larger")
	}

	// Unmarshal the data after the "//" comment string.
	if err := json.Unmarshal(data[3:], &parsed); err != nil {
		return realTimeTradingData{}, err
	}

	if len(parsed) == 0 {
		return realTimeTradingData{}, errors.New("expected at least one entry")
	}

	timestamp, err := time.Parse("2006-01-02T15:04:05Z", parsed[0].Lt_dts)
	if err != nil {
		return realTimeTradingData{}, err
	}

	price, err := strconv.ParseFloat(parsed[0].L, 64)
	if err != nil {
		return realTimeTradingData{}, err
	}

	change, err := strconv.ParseFloat(parsed[0].C, 64)
	if err != nil {
		return realTimeTradingData{}, err
	}

	percentChange, err := strconv.ParseFloat(parsed[0].Cp, 64)
	if err != nil {
		return realTimeTradingData{}, err
	}

	return realTimeTradingData{
		timestamp:     timestamp,
		price:         price,
		change:        change,
		percentChange: percentChange,
	}, nil
}
