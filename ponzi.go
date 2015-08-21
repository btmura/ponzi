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

	"github.com/nsf/termbox-go"
)

func main() {
	if err := termbox.Init(); err != nil {
		log.Fatalf("termbox.Init: %v", err)
	}
	defer termbox.Close()

	// TODO(btmura): use a single a channel

	refreshTimeChannel := make(chan *time.Time, 1)
	refreshTimeChannel <- nil

	symbols := []string{
		"SPY",
		"MO",
		"XOM",
		"DIG",
	}

	priceChannels := make(map[string]chan tradingSession)
	for _, symbol := range symbols {
		if _, ok := priceChannels[symbol]; !ok {
			ch := make(chan tradingSession, 1)
			ch <- tradingSession{}
			priceChannels[symbol] = ch
		}
	}

	go func() {
		for {
			rt := time.Now()

			for symbol, ch := range priceChannels {
				p := tradingSession{}

				th, err := getTradingHistory(symbol, time.Now().Add(time.Hour*24*5), time.Now())
				switch {
				case err != nil:
					log.Printf("getTradingHistory(%s): %v", symbol, err)

				case len(th) == 0:
					log.Printf("no tradingSession data for %s", symbol)

				default:
					sort.Reverse(th)
					p = th[0]
				}

				<-ch
				ch <- p
			}

			<-refreshTimeChannel
			refreshTimeChannel <- &rt

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

		rt := <-refreshTimeChannel
		if rt != nil {
			printTerm(0, 0, termbox.ColorDefault, termbox.ColorDefault, rt.Format("1/2/06 3:4 PM"))
		}
		refreshTimeChannel <- rt

		for i, symbol := range symbols {
			p := <-priceChannels[symbol]

			var fg termbox.Attribute
			switch {
			case p.close > p.open:
				fg = termbox.ColorGreen

			case p.open > p.close:
				fg = termbox.ColorRed

			default:
				fg = termbox.ColorDefault
			}

			printTerm(0, i+1, fg, termbox.ColorDefault, "%-10s %-10s %10.2f %+10.2f", symbol, p.date.Format("1/2/06"), p.close, p.close-p.open)
			priceChannels[symbol] <- p
		}

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

type tradingSession struct {
	date   time.Time
	open   float64
	high   float64
	low    float64
	close  float64
	volume int64
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

	var th []tradingSession
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

	return th, nil
}
