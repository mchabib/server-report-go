package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type telegramClient struct {
	token string
	http  *http.Client
}

func newTelegramClient(token string) *telegramClient {
	return &telegramClient{
		token: token,
		http:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *telegramClient) sendMessage(chatID, text string) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)

	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "Markdown")

	resp, err := t.http.PostForm(endpoint, form)
	if err != nil {
		return fmt.Errorf("request gagal: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram API HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
