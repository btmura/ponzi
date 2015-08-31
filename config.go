package main

import (
	"encoding/json"
	"os"
	"os/user"
	"path"
)

// config has the user's saved stocks.
type config struct {
	// Stocks are the config's stocks. Capitalized for JSON decoding.
	Stocks []configStock
}

// configStock represents a single user's stock.
type configStock struct {
	// Symbol is the stock's symbol. Capitalized for JSON decoding.
	Symbol string
}

func loadStockData() (*stockData, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path.Join(u.HomeDir, ".ponzi"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if os.IsNotExist(err) {
		return &stockData{}, nil
	}
	defer file.Close()

	cfg := config{}
	d := json.NewDecoder(file)
	if err := d.Decode(&cfg); err != nil {
		return nil, err
	}

	sd := stockData{}
	for _, s := range cfg.Stocks {
		sd.stocks = append(sd.stocks, stock{
			symbol: s.Symbol,
		})
	}
	return &sd, nil
}
