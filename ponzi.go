package main

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/nsf/termbox-go"
)

type stockData struct {
	// Embedded mutex that guards the stockData struct.
	sync.RWMutex

	// refreshTime is when the data was last refreshed.
	refreshTime time.Time

	// tradingDates is a chronological set of times.
	tradingDates []time.Time

	stocks []stock
}

type stock struct {
	symbol            string
	tradingHistory    stockTradingHistory
	tradingSessionMap map[time.Time]stockTradingSession
}

type stockTradingHistory []stockTradingSession

type stockTradingSession struct {
	date          time.Time
	close         float64
	volume        int64
	change        float64
	percentChange float64
}

func main() {
	if err := termbox.Init(); err != nil {
		log.Fatalf("termbox.Init: %v", err)
	}
	defer termbox.Close()

	rand.Seed(time.Now().Unix())

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
			refreshStockData(sd)

			// Signal termbox to repaint by queuing an interrupt event.
			termbox.Interrupt()

			// Sleep a bit till the next refresh.
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
			x, y := 0, 5+i*5
			fg = termbox.ColorDefault

			print(x, y, "%[1]*s", symbolWidth, s.symbol)
			x = x + symbolWidth

			for _, td := range tradingDates {
				if x+tsCellWidth > w {
					break
				}

				if ts, ok := s.tradingSessionMap[td]; ok {
					fg = termbox.ColorDefault

					// Print price and volume in default color.
					print(x, y, "%[1]*.2f", tsCellWidth, ts.close)
					print(x, y+3, "%[1]*s", tsCellWidth, shortenInt(ts.volume))

					switch {
					case ts.change > 0:
						fg = termbox.ColorGreen

					case ts.change < 0:
						fg = termbox.ColorRed

					default:
						fg = termbox.ColorDefault
					}

					// Print change and % change in green or red.
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

func refreshStockData(sd *stockData) {
	end := time.Now()
	start := end.Add(-time.Hour * 24 * 30)

	// Map from symbol to tradingHistory channel.
	scm := map[string]chan tradingHistory{}

	// Map from symbol to realTimeTradingData channel.
	rcm := map[string]chan realTimeTradingData{}

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

		// Launch a go routine that will stuff the realTimeTradingData into the channel.
		rch := make(chan realTimeTradingData)
		rcm[s.symbol] = rch
		go func(symbol string, rch chan realTimeTradingData) {
			rd, err := getRealTimeTradingData(symbol)
			if err != nil {
				log.Printf("getRealTimeTradingData(%s): %v", symbol, err)
			}
			rch <- rd
		}(s.symbol, rch)
	}
	sd.RUnlock()

	// Record the unique trading dates for all data.
	var dates sortableTimes
	tdm := map[time.Time]bool{}

	// Extract the tradingHistory from each channel into a new map.
	thm := map[string]stockTradingHistory{}
	tsm := map[string]map[time.Time]stockTradingSession{}
	for symbol, ch := range scm {
		// TODO(btmura): detect error value from channel
		thm[symbol] = convertTradingHistory(<-ch)
		for _, ts := range thm[symbol] {
			if _, ok := tsm[symbol]; !ok {
				tsm[symbol] = map[time.Time]stockTradingSession{}
			}
			tsm[symbol][ts.date] = ts
			if !tdm[ts.date] {
				tdm[ts.date] = true
				dates = append(dates, ts.date)
			}
		}
	}

	// Extract the realTimeTradingData from each channel.
	for symbol, rch := range rcm {

		// TODO(btmura): detect error value from channel
		rd := <-rch

		ts := stockTradingSession{
			date:          time.Date(rd.timestamp.Year(), rd.timestamp.Month(), rd.timestamp.Day(), 0, 0, 0, 0, rd.timestamp.Location()),
			close:         rd.price,
			change:        rd.change,
			percentChange: rd.percentChange,
		}

		if _, ok := tsm[symbol][ts.date]; !ok {
			thm[symbol] = append([]stockTradingSession{ts}, thm[symbol]...)

			if _, ok := tsm[symbol]; !ok {
				tsm[symbol] = map[time.Time]stockTradingSession{}
			}
			tsm[symbol][ts.date] = ts
			if !tdm[ts.date] {
				tdm[ts.date] = true
				dates = append(dates, ts.date)
			}
		}
	}

	// Sort the trading dates with most recent at the back.
	sort.Sort(dates)

	// Acquire a write lock and write the updated data.
	sd.Lock()
	sd.refreshTime = time.Now()
	sd.tradingDates = dates
	for i, s := range sd.stocks {
		sd.stocks[i].tradingHistory = thm[s.symbol]
		sd.stocks[i].tradingSessionMap = tsm[s.symbol]
	}
	sd.Unlock()
}

func convertTradingHistory(th tradingHistory) stockTradingHistory {
	var sth stockTradingHistory
	for _, ts := range th {
		sth = append(sth, stockTradingSession{
			date:   ts.date,
			close:  ts.close,
			volume: ts.volume,
		})
	}

	// Calculate the price change which is today's minus yesterday's close.
	for i := range sth {
		if i+1 < len(sth) {
			sth[i].change = sth[i].close - sth[i+1].close
			sth[i].percentChange = sth[i].change / sth[i+1].close * 100.0
		}
	}

	return sth
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

// shortenInt shortens larger numbers and appends a quantity suffix.
func shortenInt(val int64) string {
	switch {
	case val/1e6 > 0:
		return fmt.Sprintf("%dM", val/1e6)
	case val/1e3 > 0:
		return fmt.Sprintf("%dK", val/1e3)
	default:
		return strconv.FormatInt(val, 10)
	}
}
