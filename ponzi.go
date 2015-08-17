package main

import (
	"log"

	"github.com/nsf/termbox-go"
)

func main() {
	if err := termbox.Init(); err != nil {
		log.Fatalf("termbox.Init: %v", err)
	}

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

	defer termbox.Close()
}
