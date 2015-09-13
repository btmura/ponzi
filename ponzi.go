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

	// tsColumnWidth is the width of the columns that have trading session data.
	tsColumnWidth = 8

	// tsColumnHeight is the height of the rows that have trading session data.
	tsColumnHeight = 4

	// padding is the amount of padding in between cells.
	padding = 1

	// colorCount is the number of color steps a price change can have.
	colorCount = 5
)

var (
	// positiveColors are background colors for positive price changes. Requires 256 colors.
	positiveColors = [colorCount]termbox.Attribute{
		termbox.Attribute(23),
		termbox.Attribute(29),
		termbox.Attribute(35),
		termbox.Attribute(41),
		termbox.Attribute(47),
	}

	// negativeColors are background colors for negative price changes. Requires 256 colors.
	negativeColors = [colorCount]termbox.Attribute{
		termbox.Attribute(53),
		termbox.Attribute(89),
		termbox.Attribute(125),
		termbox.Attribute(161),
		termbox.Attribute(197),
	}

	// colorLevels is a slice of percentages at which colors change.
	colorLevels = [colorCount]float64{
		0.0,
		0.05,
		0.1,
		0.25,
		0.5,
	}

	// weekdayColors are background colors for the weekdays. Requires 256 colors.
	weekdayColors = map[time.Weekday]termbox.Attribute{
		time.Monday:    termbox.Attribute(233),
		time.Tuesday:   termbox.Attribute(234),
		time.Wednesday: termbox.Attribute(235),
		time.Thursday:  termbox.Attribute(236),
		time.Friday:    termbox.Attribute(237),
	}

	// placeholderColor is the background color for a cell with no data.
	placeholderColor = termbox.Attribute(234)

	// newYorkLoc is the New York timezone used to determine market hours.
	newYorkLoc *time.Location = mustLoadLocation("America/New_York")
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

	// Set to InputAlt so that ESC + Key enables the ModAlt flag for EventKey events.
	// ModAlt does not mean the ALT key as typically expected.
	termbox.SetInputMode(termbox.InputAlt)

	// Attempt to enable 256 color mode.
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
	symbolOffset := 0

	// Launch a go routine to periodically refresh the stock data.
	go func() {
		// refreshDuration is the duration until the next refresh.
		var refreshDuration time.Duration

		// getNextRefreshDuration returns a duration from now till the next refresh.
		getNextRefreshDuration := func() time.Duration {
			// Refresh at the top of the hour to be predictable.
			now := time.Now()
			nextRefreshTime := now.Add(1 * time.Hour).Truncate(time.Hour)
			return nextRefreshTime.Sub(now)
		}

		// refresh refreshes the stock data, repaints the screen, and calculates the next duration.
		refresh := func() {
			refreshStockData(sd, "", false)

			// Signal termbox to repaint by queuing an interrupt event.
			termbox.Interrupt()

			// Calculate the next refresh duration at the top of the hour.
			refreshDuration = getNextRefreshDuration()
		}

		// Do an initial refresh of the data.
		refresh()

		// Loop forever and perodically refresh.
		for {
			select {
			case <-time.After(refreshDuration):
				refresh()

			case <-time.After(5 * time.Minute):
				if isMarketHours() {
					refreshStockData(sd, "", true)
					termbox.Interrupt()
				}
			}
		}
	}()

loop:
	for {
		if err := termbox.Clear(termbox.ColorDefault, termbox.ColorDefault); err != nil {
			log.Fatalf("termbox.Clear: %v", err)
		}

		fg, bg := termbox.ColorDefault, termbox.ColorDefault
		w, h := termbox.Size()

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

		// Print out the dates at the top.
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

		// startY is the row after the refresh time(1) + padding(1) + date(2) + padding(1)
		const startY = 5

		// getY gets the row's top y.
		getY := func(row int) int {
			return startY + (tsColumnHeight+padding)*row
		}

		// Adjust the offset so that the selectedIndex is visible.
		for getY(selectedIndex-symbolOffset) < startY {
			symbolOffset--
		}
		for getY(selectedIndex-symbolOffset+1) > h {
			symbolOffset++
		}

		// Print out the symbols and the trading session cells.
		for i, s := range sd.stocks[symbolOffset:] {
			x, y := padding, getY(i)
			if y+tsColumnHeight+padding > h {
				break
			}

			if i+symbolOffset == selectedIndex {
				fg = termbox.ColorYellow
			} else {
				fg = termbox.ColorDefault
			}
			bg = termbox.ColorDefault

			print(x, y, "%[1]*s", symbolColumnWidth, s.symbol)
			x = x + symbolColumnWidth + padding

			for _, td := range tradingDates {
				if x+tsColumnWidth+padding > w {
					break
				}

				if ts, ok := s.tradingSessionMap[td]; ok {
					fg = termbox.ColorDefault

					c := 0
					absChange := math.Abs(ts.percentChange)
					for ; c < len(colorLevels)-1; c++ {
						if absChange < colorLevels[c+1] {
							break
						}
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
				} else {
					bg = placeholderColor
					for i := 0; i < 4; i++ {
						print(x, y+i, strings.Repeat(" ", tsColumnWidth))
					}
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
			case termbox.KeyCtrlC, termbox.KeyCtrlD:
				break loop

			case termbox.KeyCtrlR:
				refreshStockData(sd, "", false)

			// TODO(btmura): remove code duplication with KeyArrowDown.
			case termbox.KeyArrowUp:
				sd.Lock()
				if len(sd.stocks) > 0 {
					swapIndex := selectedIndex - 1
					if swapIndex < 0 {
						swapIndex = len(sd.stocks) - 1
					}
					if ev.Mod == termbox.ModAlt {
						sd.stocks[selectedIndex], sd.stocks[swapIndex] = sd.stocks[swapIndex], sd.stocks[selectedIndex]
						saveStockData(sd)
					}
					selectedIndex = swapIndex
				}
				sd.Unlock()

			case termbox.KeyArrowDown:
				sd.Lock()
				if len(sd.stocks) > 0 {
					swapIndex := selectedIndex + 1
					if swapIndex == len(sd.stocks) {
						swapIndex = 0
					}
					if ev.Mod == termbox.ModAlt {
						sd.stocks[selectedIndex], sd.stocks[swapIndex] = sd.stocks[swapIndex], sd.stocks[selectedIndex]
						saveStockData(sd)
					}
					selectedIndex = swapIndex
				}
				sd.Unlock()

			case termbox.KeyEnter:
				if inputSymbol != "" {
					sd.Lock()

					// Expand the slice, shift from selected index, and insert into the middle.
					sd.stocks = append(sd.stocks, stock{})
					copy(sd.stocks[selectedIndex+1:], sd.stocks[selectedIndex:])
					sd.stocks[selectedIndex] = stock{symbol: inputSymbol}

					saveStockData(sd)
					sd.Unlock()

					// Get initial data for the new stock.
					refreshStockData(sd, inputSymbol, false)
					inputSymbol = ""
				}

			case termbox.KeyDelete:
				sd.Lock()
				if len(sd.stocks) > 0 {
					sd.stocks = append(sd.stocks[:selectedIndex], sd.stocks[selectedIndex+1:]...)
					saveStockData(sd)
					if selectedIndex-1 >= 0 {
						selectedIndex--
					}
				}
				sd.Unlock()

			case termbox.KeyBackspace, termbox.KeyBackspace2:
				if len(inputSymbol) > 0 {
					inputSymbol = inputSymbol[:len(inputSymbol)-1]
				}

			default:
				inputSymbol += strings.ToUpper(string(ev.Ch))
			}
		}
	}
}

func refreshStockData(sd *stockData, oneSymbol string, onlyRealTimeData bool) {
	end := time.Now()
	start := end.Add(-time.Hour * 24 * 30)

	// Map from symbol to tradingSessions channel.
	scm := map[string]chan []tradingSession{}

	// Collect the symbols for a batch call to get the real time trading data.
	var symbols []string

	// queueRequest launches a go routine to get trading data for a symbol.
	queueRequest := func(newSymbol string) {
		// Avoid making redundant requests.
		if _, ok := scm[newSymbol]; ok {
			return
		}

		symbols = append(symbols, newSymbol)

		// Launch a go routine that will stuff the tradingSessions into the channel.
		ch := make(chan []tradingSession)
		scm[newSymbol] = ch
		if onlyRealTimeData {
			close(ch)
			return
		}

		go func(symbol string, ch chan []tradingSession) {
			tss, err := getTradingSessions(symbol, start, end)
			if err != nil {
				log.Printf("getTradingSessions(%s): %v", symbol, err)
			}
			ch <- tss
		}(newSymbol, ch)
	}

	// Acquire a read lock to get the symbols and launch a go routine per symbol.
	if oneSymbol != "" {
		queueRequest(oneSymbol)
	} else {
		sd.RLock()
		for _, s := range sd.stocks {
			queueRequest(s.symbol)
		}
		sd.RUnlock()
	}

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

	// dates is the unique trading dates of all the data.
	var dates sortableTimes

	// tdm and addDate should be used to update and keep the date set unique.
	tdm := map[time.Time]bool{}
	addDate := func(date time.Time) {
		if !tdm[date] {
			tdm[date] = true
			dates = append(dates, date)
		}
	}

	// Add the previous dates that we still have data for before waiting for responses.
	sd.RLock()
	for _, prevDate := range sd.tradingDates {
		addDate(prevDate)
	}
	sd.RUnlock()

	// Extract the tradingHistory from each channel into a new map.
	tsm := map[string]map[time.Time]stockTradingSession{}
	for symbol, ch := range scm {
		// TODO(btmura): detect error value from channel
		for _, ts := range convertTradingSessions(<-ch) {
			if _, ok := tsm[symbol]; !ok {
				tsm[symbol] = map[time.Time]stockTradingSession{}
			}
			tsm[symbol][ts.date] = ts
			addDate(ts.date)
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
			addDate(ts.date)
		}
	}

	// Sort the trading dates with most recent at the back.
	sort.Sort(dates)

	// Acquire a write lock and write the updated data.
	sd.Lock()
	sd.refreshTime = time.Now()
	sd.tradingDates = dates
	for i, s := range sd.stocks {
		if sd.stocks[i].tradingSessionMap == nil {
			sd.stocks[i].tradingSessionMap = map[time.Time]stockTradingSession{}
		}
		for date, ts := range tsm[s.symbol] {
			sd.stocks[i].tradingSessionMap[date] = ts
		}
	}
	sd.Unlock()
}

func convertTradingSessions(tss []tradingSession) []stockTradingSession {
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

func saveStockData(sd *stockData) {
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
}

func isMarketHours() bool {
	now := time.Now().In(newYorkLoc)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return false
	}

	opening := time.Date(now.Year(), now.Month(), now.Day(), 9, 30, 0, 0, newYorkLoc)
	if now.Before(opening) {
		return false
	}

	closing := time.Date(now.Year(), now.Month(), now.Day(), 12+4, 0, 0, 0, newYorkLoc)
	if now.After(closing) {
		return false
	}

	return true
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(fmt.Sprintf("time.LoadLocation: %v", err))
	}
	return loc
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
