package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nsf/termbox-go"
)

const (
	// symbolColumnWidth is the width of the leftmost column with the symbols.
	symbolColumnWidth = 5

	// tsColumnWidth is the width of the middle columns that have trading session data.
	tsColumnWidth = 8

	// padding is the amount of padding around all edges and in between cells.
	padding = 1
)

var (
	// positiveColors are background colors for positive price changes. Requires 256 colors.
	positiveColors = [5]termbox.Attribute{
		termbox.Attribute(23),
		termbox.Attribute(29),
		termbox.Attribute(35),
		termbox.Attribute(41),
		termbox.Attribute(47),
	}

	// negativeColors are background colors for negative price changes. Requires 256 colors.
	negativeColors = [5]termbox.Attribute{
		termbox.Attribute(53),
		termbox.Attribute(89),
		termbox.Attribute(125),
		termbox.Attribute(161),
		termbox.Attribute(197),
	}

	// weekdayColors are background colors for the weekdays. Requires 256 colors.
	weekdayColors = map[time.Weekday]termbox.Attribute{
		time.Monday:    termbox.Attribute(233),
		time.Tuesday:   termbox.Attribute(234),
		time.Wednesday: termbox.Attribute(234),
		time.Thursday:  termbox.Attribute(235),
		time.Friday:    termbox.Attribute(236),
	}
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

	has256Colors := termbox.SetOutputMode(termbox.Output256) == termbox.Output256

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("loadConfig: %v", err)
	}

	sd := &stockData{}
	for _, cs := range cfg.Stocks {
		sd.stocks = append(sd.stocks, stock{
			symbol: cs.Symbol,
		})
	}

	inputSymbol := ""
	selectedIndex := 0

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

		if !sd.refreshTime.IsZero() {
			print(0, 0, sd.refreshTime.Format("1/2/06 3:04 PM"))
		}

		print(20, 0, inputSymbol)

		// Trim down trading dates to what fits the screen.
		tsColumnCount := (w - symbolColumnWidth - padding) / (tsColumnWidth + padding)
		if tsColumnCount > len(sd.tradingDates) {
			tsColumnCount = len(sd.tradingDates)
		}
		tradingDates := sd.tradingDates[len(sd.tradingDates)-tsColumnCount:]

		x := symbolColumnWidth + padding*2
		for _, td := range tradingDates {
			if x+tsColumnWidth+padding > w {
				break
			}

			switch {
			case has256Colors:
				bg = weekdayColors[td.Weekday()]
			default:
				bg = termbox.ColorDefault
			}

			print(x, 2, "%[1]*s", tsColumnWidth, td.Format("1/2"))
			print(x, 3, "%[1]*s", tsColumnWidth, td.Format("Mon"))
			x = x + tsColumnWidth + padding
		}

		for i, s := range sd.stocks {
			x, y := padding, 5+i*5
			bg = termbox.ColorDefault

			if i == selectedIndex {
				fg = termbox.ColorYellow
			} else {
				fg = termbox.ColorDefault
			}

			print(x, y, "%[1]*s", symbolColumnWidth, s.symbol)
			x = x + symbolColumnWidth + padding

			for _, td := range tradingDates {
				if x+tsColumnWidth+padding > w {
					break
				}

				if ts, ok := s.tradingSessionMap[td]; ok {
					fg = termbox.ColorDefault

					abs := math.Abs(ts.percentChange)
					c := 0
					switch {
					case abs > 0.6:
						c = 5
					case abs > 0.4:
						c = 4
					case abs > 0.2:
						c = 3
					case abs > 0.1:
						c = 2
					case abs > 0.05:
						c = 1
					}

					switch {
					case has256Colors && ts.change > 0:
						bg = positiveColors[c]
					case has256Colors && ts.change < 0:
						bg = negativeColors[c]
					default:
						bg = termbox.ColorDefault
					}

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
					print(x, y+2, "%+[1]*.2f%%", tsColumnWidth-1, ts.percentChange*100.0)
				}
				x = x + tsColumnWidth + padding
			}
		}

		sd.RUnlock()

		if err := termbox.Flush(); err != nil {
			log.Fatalf("termbox.Flush: %v", err)
		}

		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyCtrlC:
				break loop

			case termbox.KeyEnter:
				if inputSymbol != "" {
					sd.Lock()
					sd.stocks = append(sd.stocks, stock{symbol: inputSymbol})
					if selectedIndex < 0 {
						selectedIndex++
					}

					cfg := config{}
					for _, s := range sd.stocks {
						cfg.Stocks = append(cfg.Stocks, configStock{
							Symbol: s.symbol,
						})
					}
					go func() {
						if err := saveConfig(cfg); err != nil {
							log.Printf("saveConfig: %v", err)
						}
					}()

					sd.Unlock()
					inputSymbol = ""

				}

			case termbox.KeyDelete:
				sd.Lock()
				sd.stocks = append(sd.stocks[:selectedIndex], sd.stocks[selectedIndex+1:]...)
				if selectedIndex >= len(sd.stocks) {
					selectedIndex--
				}
				sd.Unlock()

			case termbox.KeyBackspace, termbox.KeyBackspace2:
				if len(inputSymbol) > 0 {
					inputSymbol = inputSymbol[:len(inputSymbol)-1]
				}

			case termbox.KeyArrowUp:
				sd.RLock()
				if selectedIndex-1 >= 0 {
					selectedIndex--
				}
				sd.RUnlock()

			case termbox.KeyArrowDown:
				sd.RLock()
				if selectedIndex+1 < len(sd.stocks) {
					selectedIndex++
				}
				sd.RUnlock()

			default:
				inputSymbol += strings.ToUpper(string(ev.Ch))
			}
		}
	}
}

func refreshStockData(sd *stockData) {
	end := time.Now()
	start := end.Add(-time.Hour * 24 * 30)

	// Map from symbol to tradingSessions channel.
	scm := map[string]chan []tradingSession{}

	// Collect the symbols for a batch call to get the real time trading data.
	var symbols []string

	// Acquire a read lock to get the symbols and launch a go routine per symbol.
	sd.RLock()
	for _, s := range sd.stocks {
		// Avoid making redundant requests.
		if _, ok := scm[s.symbol]; ok {
			continue
		}

		symbols = append(symbols, s.symbol)

		// Launch a go routine that will stuff the tradingSessions into the channel.
		ch := make(chan []tradingSession)
		scm[s.symbol] = ch
		go func(symbol string, ch chan []tradingSession) {
			tss, err := getTradingSessions(symbol, start, end)
			if err != nil {
				log.Printf("getTradingSessions(%s): %v", symbol, err)
			}
			ch <- tss
		}(s.symbol, ch)
	}
	sd.RUnlock()

	// Batch get the real time trading data using the aggregated symbols.
	rc := make(chan []realTimeTradingData)
	go func() {
		if len(symbols) > 0 {
			rds, err := getRealTimeTradingData(symbols)
			if err != nil {
				log.Printf("getRealTimeTradingData(%v): %v", symbols, err)
			}
			rc <- rds
		}
		close(rc)
	}()

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

	// Extract the realTimeTradingData from the channel.
	for _, rd := range <-rc {

		// TODO(btmura): detect error value from channel

		ts := stockTradingSession{
			date:          time.Date(rd.timestamp.Year(), rd.timestamp.Month(), rd.timestamp.Day(), 0, 0, 0, 0, rd.timestamp.Location()),
			close:         rd.price,
			change:        rd.change,
			percentChange: rd.percentChange,
		}

		if _, ok := tsm[rd.symbol][ts.date]; !ok {
			if _, ok := tsm[rd.symbol]; !ok {
				tsm[rd.symbol] = map[time.Time]stockTradingSession{}
			}
			tsm[rd.symbol][ts.date] = ts
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
			sts[i].percentChange = sts[i].change / sts[i+1].close
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
