package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Client maps a Prometheus federation label to a Telegram destination.
type Client struct {
	Label          string `yaml:"label"`
	Name           string `yaml:"name"`
	TelegramChatID string `yaml:"telegram_chat_id"`
}

type clientsFile struct {
	Clients []Client `yaml:"clients"`
}

func clientsFilePath() string {
	return filepath.Join(binDir(), "clients.yaml")
}

// loadClients reads and validates clients.yaml. Every entry must have all
// three fields populated and labels must be unique — this file controls
// where Telegram notifications go, so a bad edit should fail loudly here
// rather than silently misroute or drop a report at send time.
func loadClients(path string) ([]Client, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gagal baca %s: %w", path, err)
	}

	var cfg clientsFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("gagal parse %s: %w", path, err)
	}

	if len(cfg.Clients) == 0 {
		return nil, fmt.Errorf("%s tidak berisi client sama sekali", path)
	}

	seen := make(map[string]bool, len(cfg.Clients))
	for i, c := range cfg.Clients {
		if c.Label == "" {
			return nil, fmt.Errorf("%s: entry ke-%d field 'label' kosong", path, i+1)
		}
		if c.Name == "" {
			return nil, fmt.Errorf("%s: client %q field 'name' kosong", path, c.Label)
		}
		if c.TelegramChatID == "" {
			return nil, fmt.Errorf("%s: client %q field 'telegram_chat_id' kosong", path, c.Label)
		}
		if seen[c.Label] {
			return nil, fmt.Errorf("%s: label client %q duplikat", path, c.Label)
		}
		seen[c.Label] = true
	}

	return cfg.Clients, nil
}
