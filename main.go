package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	envPath := filepath.Join(binDir(), ".env")
	if err := loadDotEnv(envPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: gagal load %s: %v\n", envPath, err)
	}

	prometheusURL := os.Getenv("PROMETHEUS_URL")
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")

	if prometheusURL == "" || botToken == "" {
		fmt.Fprintln(os.Stderr, "error: PROMETHEUS_URL dan TELEGRAM_BOT_TOKEN wajib diisi (lewat .env atau environment variable)")
		os.Exit(1)
	}

	clients, err := loadClients(clientsFilePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	prom := newPromClient(prometheusURL)
	tg := newTelegramClient(botToken)
	now := time.Now()

	successCount, failCount := 0, 0

	for _, c := range clients {
		if err := processClient(prom, tg, c, now); err != nil {
			fmt.Fprintf(os.Stderr, "warning: client %q gagal: %v\n", c.Label, err)
			failCount++
			continue
		}
		fmt.Printf("client %q: laporan terkirim\n", c.Label)
		successCount++
	}

	fmt.Printf("\nringkasan: %d sukses, %d gagal (dari %d client)\n", successCount, failCount, len(clients))

	if failCount > 0 {
		os.Exit(1)
	}
}

// processClient fetches metrics and sends the Telegram report for a
// single client. Errors here are handled by the caller as a warning so
// one bad client never stops the rest of the run.
func processClient(prom *promClient, tg *telegramClient, c Client, now time.Time) error {
	metrics, err := fetchClientMetrics(prom, c.Label)
	if err != nil {
		return fmt.Errorf("query prometheus: %w", err)
	}

	text := buildReport(c.Name, metrics, now)

	if err := tg.sendMessage(c.TelegramChatID, text); err != nil {
		return fmt.Errorf("kirim telegram: %w", err)
	}

	return nil
}
