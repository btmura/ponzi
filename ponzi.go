package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nsf/termbox-go"
)

var (
	// dataSource is a flag to set what data source to use.
	dataSource = flag.String("data_source", string(google), "Data source to get quotes. Values: google, yahoo")

	// getTradingSessions is the tradingSessionFunc set by the dataSource flag.
	getTradingSessions tradingSessionFunc
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

	dow    stockTradingSession
	sap    stockTradingSession
	nasdaq stockTradingSession
}

var (
	dowSymbol    = ".DJI"
	sapSymbol    = ".INX"
	nasdaqSymbol = ".IXIC"
	indexSymbols = []string{
		dowSymbol,
		sapSymbol,
		nasdaqSymbol,
	}
)

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
	var err error

	// Parse flags. Do initialization that we can do before termbox starts.
	flag.Parse()

	getTradingSessions, err = getTradingSessionFunc(tradingSessionSource(*dataSource))
	if err != nil {
		log.Fatalf("getTradingSessionFunc: %v", err)
	}

	// Redirect the logger since termbox will cover the screen.
	logFile, err := initLogger()
	if err != nil {
		log.Fatalf("initLogger: %v", err)
	}
	defer logFile.Close()

	// Try to initialize termbox now.
	if err := termbox.Init(); err != nil {
		log.Fatalf("termbox.Init: %v", err)
	}
	defer termbox.Close()

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
			refreshStockData(sd, "")

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
			const indexSymbolWidth = 6
			print(0, 0, "%s %.2f %+.2f %+.2f%% "+
				"%s %.2f %+.2f %+.2f%% "+
				"%s %.2f %+.2f %+.2f%%",

				"DOW",
				sd.dow.close,
				sd.dow.change,
				sd.dow.percentChange*100.0,

				"S&P",
				sd.sap.close,
				sd.sap.change,
				sd.sap.percentChange*100.0,

				"NASDAQ",
				sd.nasdaq.close,
				sd.nasdaq.change,
				sd.nasdaq.percentChange*100.0)

			s := sd.refreshTime.Format("1/2/06 3:04 PM")
			print(w-len(s), 0, s)
		}

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
				fg = termbox.ColorYellow | termbox.AttrBold
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

		// Print out the input symbol in the center of the screen.
		if inputSymbol != "" {
			fg, bg = termbox.ColorWhite, termbox.ColorBlue
			ps := strings.Repeat(" ", padding)
			pr := ps + strings.Repeat(" ", len(inputSymbol)) + ps
			cx, cy := w/2-len(pr)/2, h/2-1
			print(cx, cy-1, pr)
			print(cx, cy, ps+inputSymbol+ps)
			print(cx, cy+1, pr)
		}

		if err := termbox.Flush(); err != nil {
			log.Fatalf("termbox.Flush: %v", err)
		}

		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyCtrlC, termbox.KeyCtrlD:
				break loop

			case termbox.KeyCtrlR, termbox.KeyF5:
				refreshStockData(sd, "")

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
					refreshStockData(sd, inputSymbol)
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

func refreshStockData(sd *stockData, oneSymbol string) {
	// start and end times to set on the data requests.
	var (
		end   = midnight(time.Now().In(newYorkLoc))
		start = end.Add(-30 * 24 * time.Hour)
	)

	// Map from symbol to tradingSessions channel.
	chm := map[string]chan []tradingSession{}

	// Collect the symbols for a batch call to get the real time trading data.
	var symbols []string

	// launchRequest records the requested symbol and launches a go routine for the data.
	launchRequest := func(newSymbol string) {
		// Avoid making redundant requests.
		if _, ok := chm[newSymbol]; ok {
			return
		}

		symbols = append(symbols, newSymbol)

		// Launch a go routine that will stuff the tradingSessions into the channel.
		ch := make(chan []tradingSession)
		chm[newSymbol] = ch
		go func(symbol string, ch chan []tradingSession) {
			tss, err := getTradingSessions(symbol, start, end)
			if err != nil {
				log.Printf("getTradingSessions(%s): %v", symbol, err)
			}
			ch <- tss
		}(newSymbol, ch)
	}

	// Launch requests for the specific symbol or all the symbols.
	if oneSymbol != "" {
		launchRequest(oneSymbol)
	} else {
		sd.RLock()
		for _, s := range sd.stocks {
			launchRequest(s.symbol)
		}
		sd.RUnlock()
	}

	// Get the live trading sessions for the stocks.
	ch := make(chan []liveTradingSession)
	go func(ch chan []liveTradingSession) {
		tss, err := getLiveTradingSessions(symbols)
		if err != nil {
			log.Printf("getLiveTradingSessions: %v", err)
		}
		ch <- tss
	}(ch)

	// Get the live trading sessions for the major indices.
	ich := make(chan []liveTradingSession)
	go func(ch chan []liveTradingSession) {
		tss, err := getLiveTradingSessions(indexSymbols)
		if err != nil {
			log.Printf("getLiveTradingSessions: %v", err)
		}
		ch <- tss
	}(ich)

	// dates is the sorted set of trading dates that will be shown at the top.
	// Use addDate to correctly modify the dates.
	var (
		dates   sortableTimes
		dm      = map[time.Time]bool{}
		addDate = func(date time.Time) {
			if !dm[date] {
				dm[date] = true
				dates = append(dates, date)
			}
		}
	)

	// Add the previous dates that we still have data for before waiting for responses.
	sd.RLock()
	for _, prevDate := range sd.tradingDates {
		addDate(prevDate)
	}
	sd.RUnlock()

	// tsm is map from date to trading data that will be used for the cells.
	// Use addTradingSession to correctly modify tsm.
	var (
		tsm               = map[string]map[time.Time]stockTradingSession{}
		addTradingSession = func(symbol string, ts stockTradingSession) {
			if _, ok := tsm[symbol]; !ok {
				tsm[symbol] = map[time.Time]stockTradingSession{}
			}
			tsm[symbol][ts.date] = ts
			addDate(ts.date)
		}
	)

	// Extract the trading sessions from each channel and put them into the map.
	for symbol, ch := range chm {
		// TODO(btmura): detect error value from channel
		for _, ts := range convertTradingSessions(<-ch) {
			addTradingSession(symbol, ts)
		}
	}

	// Extract the live trading sessions and put them into the map.
	for symbol, ts := range convertLiveTradingSessions(<-ch) {
		addTradingSession(symbol, ts)
	}

	// Extract the live trading sessions for the indices.
	im := convertLiveTradingSessions(<-ich)

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
	sd.dow = im[dowSymbol]
	sd.sap = im[sapSymbol]
	sd.nasdaq = im[nasdaqSymbol]
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

func convertLiveTradingSessions(lts []liveTradingSession) map[string]stockTradingSession {
	m := map[string]stockTradingSession{}
	for _, lt := range lts {
		m[lt.symbol] = stockTradingSession{
			date:          midnight(lt.timestamp),
			close:         lt.price,
			change:        lt.change,
			percentChange: lt.percentChange,
		}
	}
	return m
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
