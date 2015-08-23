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

	tradingDates tradingDates

	stocks []stock
}

type stock struct {
	symbol            string
	tradingHistory    tradingHistory
	tradingSessionMap map[time.Time]tradingSession
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
			stock{symbol: "CEF"},
			stock{symbol: "GOOG"},
			stock{symbol: "AAPL"},
		},
	}

	// Launch a go routine to periodically refresh the stock data.
	go func() {
		for {
			end := time.Now()
			start := end.Add(-time.Hour * 24 * 5)

			// Map from symbol to tradingHistory channel.
			scm := map[string]chan tradingHistory{}

			// Acquire a read lock to get the symbols and launch a go routine per symbol.
			sd.RLock()
			for _, s := range sd.stocks {
				// Avoid making redundant requests.
				if _, ok := scm[s.symbol]; ok {
					continue
				}

				// Launch a go routine that will stuff the tradingHistory into the channel.
				ch := make(chan tradingHistory)
				scm[s.symbol] = ch
				go func(symbol string, ch chan tradingHistory) {
					th, err := getTradingHistory(symbol, start, end)
					if err != nil {
						log.Printf("getTradingHistory(%s): %v", symbol, err)
					}
					ch <- th
				}(s.symbol, ch)
			}
			sd.RUnlock()

			// Record the unique trading dates for all data.
			var td tradingDates
			tdm := map[time.Time]bool{}

			// Extract the tradingHistory from each channel into a new map.
			thm := map[string]tradingHistory{}
			tsm := map[string]map[time.Time]tradingSession{}
			for symbol, ch := range scm {
				thm[symbol] = <-ch
				for _, ts := range thm[symbol] {
					if _, ok := tsm[symbol]; !ok {
						tsm[symbol] = map[time.Time]tradingSession{}
					}
					tsm[symbol][ts.date] = ts
					if !tdm[ts.date] {
						tdm[ts.date] = true
						td = append(td, ts.date)
					}
				}
			}

			// Sort the trading dates with most recent at the back.
			sort.Sort(td)

			// Acquire a write lock and write the updated data.
			sd.Lock()
			sd.refreshTime = time.Now()
			sd.tradingDates = td
			for i, s := range sd.stocks {
				sd.stocks[i].tradingHistory = thm[s.symbol]
				sd.stocks[i].tradingSessionMap = tsm[s.symbol]
			}
			sd.Unlock()

			// Signal termbox to update itself and goto sleep.
			termbox.Interrupt()
			time.Sleep(time.Hour)
		}
	}()

	const (
		symbolWidth = 6
		tsCellWidth = 10
	)

loop:
	for {
		if err := termbox.Clear(termbox.ColorDefault, termbox.ColorDefault); err != nil {
			log.Fatalf("termbox.Clear: %v", err)
		}

		fg, bg := termbox.ColorDefault, termbox.ColorDefault
		w, _ := termbox.Size()

		print := func(x, y int, format string, a ...interface{}) {
			for _, rune := range fmt.Sprintf(format, a...) {
				termbox.SetCell(x, y, rune, fg, bg)
				x++
			}
		}

		sd.RLock()

		print(0, 0, sd.refreshTime.Format("1/2/06 3:04 PM"))

		// Trim down trading dates to what fits the screen.
		tsCellCount := (w - symbolWidth) / tsCellWidth
		if tsCellCount > len(sd.tradingDates) {
			tsCellCount = len(sd.tradingDates)
		}
		tradingDates := sd.tradingDates[len(sd.tradingDates)-tsCellCount:]

		x := symbolWidth
		for _, td := range tradingDates {
			if x+tsCellWidth > w {
				break
			}
			print(x, 2, "%[1]*s", tsCellWidth, td.Format("1/2"))
			print(x, 3, "%[1]*s", tsCellWidth, td.Format("Mon"))
			x = x + tsCellWidth
		}

		for i, s := range sd.stocks {
			x, y := 0, 5+i*4
			fg = termbox.ColorDefault

			print(x, y, "%[1]*s", symbolWidth, s.symbol)
			x = x + symbolWidth

			for _, td := range tradingDates {
				if x+tsCellWidth > w {
					break
				}

				if ts, ok := s.tradingSessionMap[td]; ok {
					fg = termbox.ColorDefault

					print(x, y, "%[1]*.2f", tsCellWidth, ts.close)

					switch {
					case ts.change > 0:
						fg = termbox.ColorGreen

					case ts.change < 0:
						fg = termbox.ColorRed

					default:
						fg = termbox.ColorDefault
					}
					print(x, y+1, "%+[1]*.2f", tsCellWidth, ts.change)
					print(x, y+2, "%+[1]*.2f%%", tsCellWidth-1, ts.percentChange)
				}
				x = x + tsCellWidth
			}
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
	date          time.Time
	open          float64
	high          float64
	low           float64
	close         float64
	volume        int64
	change        float64
	percentChange float64
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

	// Calculate the price change which is today's minus yesterday's close.
	for i := range th {
		if i+1 < len(th) {
			th[i].change = th[i].close - th[i+1].close
			th[i].percentChange = th[i].change / th[i+1].close * 100.0
		}
	}

	return th, nil
}

type tradingDates []time.Time

// Len implements sort.Interface
func (td tradingDates) Len() int {
	return len(td)
}

// Less implements sort.Interface
func (td tradingDates) Less(i, j int) bool {
	return td[i].Before(td[j])
}

// Swap implements sort.Interface
func (td tradingDates) Swap(i, j int) {
	td[i], td[j] = td[j], td[i]
}
