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
	"strings"
	"time"
)

// tradingSessionSource is a source of trading sessions.
type tradingSessionSource string

// List of possible tradingSessionSource values.
const (
	google tradingSessionSource = "google"
	yahoo                       = "yahoo"
	random                      = "random"
)

// tradingSessionFunc is a function that returns tradingSessions.
type tradingSessionFunc func(symbol string, startDate, endDate time.Time) ([]tradingSession, error)

func getTradingSessionFunc(source tradingSessionSource) (tradingSessionFunc, error) {
	switch source {
	case google:
		return getTradingSessionsFromGoogle, nil
	case yahoo:
		return getTradingSessionsFromYahoo, nil
	case random:
		return getTradingSessionsFromRandom, nil
	default:
		return nil, fmt.Errorf("unrecognized value: %s", source)
	}
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

func getTradingSessionsFromRandom(symbol string, startDate, endDate time.Time) ([]tradingSession, error) {
	randomSources := []tradingSessionSource{
		google,
		yahoo,
	}
	for _, v := range rand.Perm(len(randomSources)) {
		s := randomSources[v]
		getTradingSessions, err := getTradingSessionFunc(s)
		if err != nil {
			return nil, err
		}

		tss, err := getTradingSessions(symbol, startDate, endDate)
		if err != nil {
			log.Printf("tradingFunc %s: %v", s, err)
			continue
		}
		return tss, nil
	}
	return nil, fmt.Errorf("all %d tradingFuncs failed", len(randomSources))
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

type liveTradingSession struct {
	symbol        string
	timestamp     time.Time
	price         float64
	change        float64
	percentChange float64
}

func getLiveTradingSessions(symbols []string) ([]liveTradingSession, error) {
	v := url.Values{}
	v.Set("client", "ig")
	v.Set("q", strings.Join(symbols, ","))

	u, err := url.Parse("http://www.google.com/finance/info")
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

	parsed := []struct {
		T      string // ticker symbol
		L      string // price
		C      string // change
		Cp     string // percent change
		Lt_dts string // time
	}{}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check that data has the expected "//" comment string to trim off.
	if len(data) < 3 {
		return nil, fmt.Errorf("expected data should be larger")
	}

	// Unmarshal the data after the "//" comment string.
	if err := json.Unmarshal(data[3:], &parsed); err != nil {
		return nil, err
	}

	if len(parsed) == 0 {
		return nil, errors.New("expected at least one entry")
	}

	var lts []liveTradingSession
	for _, p := range parsed {
		timestamp, err := time.Parse("2006-01-02T15:04:05Z", p.Lt_dts)
		if err != nil {
			return nil, fmt.Errorf("p: %+v timestamp: %v", p, err)
		}

		price, err := strconv.ParseFloat(p.L, 64)
		if err != nil {
			return nil, fmt.Errorf("p: %+v price: %v", p, err)
		}

		var change float64
		if p.C != "" { // C is empty after market close.
			change, err = strconv.ParseFloat(p.C, 64)
			if err != nil {
				return nil, fmt.Errorf("p: %+v change: %v", p, err)
			}
		}

		var percentChange float64
		if p.Cp != "" { // Cp is empty after market close.
			percentChange, err = strconv.ParseFloat(p.Cp, 64)
			if err != nil {
				return nil, fmt.Errorf("p: %+v percentChange: %v", p, err)
			}
			percentChange /= 100.0
		}

		lts = append(lts, liveTradingSession{
			symbol:        p.T,
			timestamp:     timestamp,
			price:         price,
			change:        change,
			percentChange: percentChange,
		})
	}

	return lts, nil
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
