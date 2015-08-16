package main

import (
	"log"

	"github.com/nsf/termbox-go"
)

func main() {
	if err := termbox.Init(); err != nil {
		log.Fatalf("termbox.Init: %v", err)
	}
	defer termbox.Close()
}
