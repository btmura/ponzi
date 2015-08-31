package main

import (
	"encoding/json"
	"os"
	"os/user"
	"path"
)

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

	type configStock struct {
		Symbol string
	}

	type configData struct {
		Stocks []configStock
	}

	cd := configData{}
	d := json.NewDecoder(file)
	if err := d.Decode(&cd); err != nil {
		return nil, err
	}

	sd := stockData{}
	for _, s := range cd.Stocks {
		sd.stocks = append(sd.stocks, stock{
			symbol: s.Symbol,
		})
	}
	return &sd, nil
}
