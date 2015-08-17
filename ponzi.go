package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/nsf/termbox-go"
)

func main() {
	pd, err := getPriceData("SPY")
	if err != nil {
		log.Fatalf("getPriceData: %v", err)
	}

	fmt.Printf("%s", pd[0].close)

	return

	if err := termbox.Init(); err != nil {
		log.Fatalf("termbox.Init: %v", err)
	}
	defer termbox.Close()

	if err := termbox.Clear(termbox.ColorDefault, termbox.ColorDefault); err != nil {
		log.Fatalf("termbox.Clear: %v", err)
	}

loop:
	for {
		termbox.SetCell(0, 0, 'S', termbox.ColorDefault, termbox.ColorDefault)
		termbox.SetCell(1, 0, 'P', termbox.ColorDefault, termbox.ColorDefault)
		termbox.SetCell(2, 0, 'Y', termbox.ColorDefault, termbox.ColorDefault)

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

type price struct {
	date   string
	open   string
	high   string
	low    string
	close  string
	volume string
}

func getPriceData(symbol string) ([]price, error) {
	resp, err := http.Get(fmt.Sprintf("http://www.google.com/finance/historical?q=%s&&output=csv", symbol))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pd []price
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
			pd = append(pd, price{
				date:   record[0],
				open:   record[1],
				high:   record[2],
				low:    record[3],
				close:  record[4],
				volume: record[5],
			})
		}
	}

	return pd, nil
}
