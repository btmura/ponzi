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

const (
	// symbolColumnWidth is the width of the leftmost column with the symbols.
	symbolColumnWidth = 6

	// tsColumnWidth is the width of the middle columns that have trading session data.
	tsColumnWidth = 10
)

type stockData struct {
	// Embedded mutex that guards the stockData struct.
	sync.RWMutex

	// refreshTime is when the data was last refreshed.
	refreshTime time.Time

	// tradingDates is the chronological set of times shown at the top.
	tradingDates []time.Time

	// stocks are stock symbols with trading session data.
	stocks []stock
}

type stock struct {
	symbol            string
	tradingSessionMap map[time.Time]stockTradingSession
}

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
		tsColumnCount := (w - symbolColumnWidth) / tsColumnWidth
		if tsColumnCount > len(sd.tradingDates) {
			tsColumnCount = len(sd.tradingDates)
		}
		tradingDates := sd.tradingDates[len(sd.tradingDates)-tsColumnCount:]

		x := symbolColumnWidth
		for _, td := range tradingDates {
			if x+tsColumnWidth > w {
				break
			}
			print(x, 2, "%[1]*s", tsColumnWidth, td.Format("1/2"))
			print(x, 3, "%[1]*s", tsColumnWidth, td.Format("Mon"))
			x = x + tsColumnWidth
		}

		for i, s := range sd.stocks {
			x, y := 0, 5+i*5
			fg = termbox.ColorDefault

			print(x, y, "%[1]*s", symbolColumnWidth, s.symbol)
			x = x + symbolColumnWidth

			for _, td := range tradingDates {
				if x+tsColumnWidth > w {
					break
				}

				if ts, ok := s.tradingSessionMap[td]; ok {
					fg = termbox.ColorDefault

					// Print price and volume in default color.
					print(x, y, "%[1]*.2f", tsColumnWidth, ts.close)
					print(x, y+3, "%[1]*s", tsColumnWidth, shortenInt(ts.volume))

					switch {
					case ts.change > 0:
						fg = termbox.ColorGreen

					case ts.change < 0:
						fg = termbox.ColorRed

					default:
						fg = termbox.ColorDefault
					}

					// Print change and % change in green or red.
					print(x, y+1, "%+[1]*.2f", tsColumnWidth, ts.change)
					print(x, y+2, "%+[1]*.2f%%", tsColumnWidth-1, ts.percentChange)
				}
				x = x + tsColumnWidth
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
	scm := map[string]chan []tradingSession{}

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
		ch := make(chan []tradingSession)
		scm[s.symbol] = ch
		go func(symbol string, ch chan []tradingSession) {
			tss, err := getTradingSessions(symbol, start, end)
			if err != nil {
				log.Printf("getTradingSessions(%s): %v", symbol, err)
			}
			ch <- tss
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
	tsm := map[string]map[time.Time]stockTradingSession{}
	for symbol, ch := range scm {
		// TODO(btmura): detect error value from channel
		for _, ts := range convertTradingHistory(<-ch) {
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
		sd.stocks[i].tradingSessionMap = tsm[s.symbol]
	}
	sd.Unlock()
}

func convertTradingHistory(tss []tradingSession) []stockTradingSession {
	var sts []stockTradingSession
	for _, ts := range tss {
		sts = append(sts, stockTradingSession{
			date:   ts.date,
			close:  ts.close,
			volume: ts.volume,
		})
	}

	// Calculate the price change which is today's minus yesterday's close.
	for i := range sts {
		if i+1 < len(sts) {
			sts[i].change = sts[i].close - sts[i+1].close
			sts[i].percentChange = sts[i].change / sts[i+1].close * 100.0
		}
	}

	return sts
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
