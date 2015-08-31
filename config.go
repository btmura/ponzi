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

func loadConfig() (config, error) {
	cfgPath, err := getUserConfigPath()
	if err != nil {
		return config{}, err
	}

	file, err := os.Open(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return config{}, err
	}
	if os.IsNotExist(err) {
		return config{}, nil
	}
	defer file.Close()

	cfg := config{}
	d := json.NewDecoder(file)
	if err := d.Decode(&cfg); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func saveConfig(cfg config) error {
	cfgPath, err := getUserConfigPath()
	if err != nil {
		return err
	}

	file, err := os.OpenFile(cfgPath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(&cfg)
}

func getUserConfigPath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return path.Join(u.HomeDir, ".ponzi"), nil
}
