package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/nsf/termbox-go"
)

func main() {
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

	if err := getPriceData("SPY"); err != nil {
		log.Fatalf("getPriceData: %v", err)
	}
}

func getPriceData(symbol string) error {
	resp, err := http.Get(fmt.Sprintf("http://www.google.com/finance/historical?q=%s&&output=csv", symbol))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("data: %s", body)
	return nil
}
