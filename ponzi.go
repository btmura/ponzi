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
	"sync"
	"time"

	"github.com/nsf/termbox-go"
)

type stockData struct {
	sync.RWMutex

	// refreshTime is when the data was last refreshed.
	refreshTime time.Time

	stocks []stock
}

type stock struct {
	symbol         string
	tradingHistory tradingHistory
}

func main() {
	if err := termbox.Init(); err != nil {
		log.Fatalf("termbox.Init: %v", err)
	}
	defer termbox.Close()

	sd := &stockData{
		stocks: []stock{
			stock{symbol: "SPY"},
			stock{symbol: "MO"},
			stock{symbol: "XOM"},
			stock{symbol: "DIG"},
		},
	}

	// Launch a go routine to periodically refresh the stock data.
	go func() {
		for {
			end := time.Now()
			start := end.Add(-time.Hour * 24 * 5)

			// Map from symbol to tradingHistory channel.
			cm := make(map[string]chan tradingHistory)

			// Acquire a read lock to get the symbols and launch a go routine per symbol.
			sd.RLock()
			for _, s := range sd.stocks {
				// Avoid making redundant requests.
				if _, ok := cm[s.symbol]; ok {
					continue
				}

				// Launch a go routine that will stuff the tradingHistory into the channel.
				ch := make(chan tradingHistory)
				cm[s.symbol] = ch
				go func(symbol string, ch chan tradingHistory) {
					th, err := getTradingHistory(symbol, start, end)
					if err != nil {
						log.Printf("getTradingHistory(%s): %v", symbol, err)
					}
					ch <- th
				}(s.symbol, ch)
			}
			sd.RUnlock()

			// Extract the tradingHistory from each channel into a new map.
			tm := make(map[string]tradingHistory)
			for symbol, ch := range cm {
				tm[symbol] = <-ch
			}

			// Acquire a write lock and write the updated data.
			sd.Lock()
			sd.refreshTime = time.Now()
			for i, s := range sd.stocks {
				sd.stocks[i].tradingHistory = tm[s.symbol]
			}
			sd.Unlock()

			// Signal termbox to update itself and goto sleep.
			termbox.Interrupt()
			time.Sleep(time.Hour)
		}
	}()

loop:
	for {
		if err := termbox.Clear(termbox.ColorDefault, termbox.ColorDefault); err != nil {
			log.Fatalf("termbox.Clear: %v", err)
		}

		printTerm := func(x, y int, fg, bg termbox.Attribute, format string, a ...interface{}) {
			for _, rune := range fmt.Sprintf(format, a...) {
				termbox.SetCell(x, y, rune, fg, bg)
				x++
			}
		}

		sd.RLock()

		printTerm(0, 0, termbox.ColorDefault, termbox.ColorDefault, sd.refreshTime.Format("1/2/06 3:04 PM"))

		for i, s := range sd.stocks {
			if len(s.tradingHistory) == 0 {
				continue
			}

			ts := s.tradingHistory[0]

			var fg termbox.Attribute
			switch {
			case ts.close > ts.open:
				fg = termbox.ColorGreen

			case ts.open > ts.close:
				fg = termbox.ColorRed

			default:
				fg = termbox.ColorDefault
			}

			printTerm(0, i+1, fg, termbox.ColorDefault, "%-10s %-10s %10.2f %+10.2f", s.symbol, ts.date.Format("1/2/06"), ts.close, ts.close-ts.open)
		}

		sd.RUnlock()

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

type tradingSession struct {
	date   time.Time
	open   float64
	high   float64
	low    float64
	close  float64
	volume int64
}

func getTradingHistory(symbol string, startDate time.Time, endDate time.Time) (tradingHistory, error) {
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
