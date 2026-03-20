package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// TradeEvent mirrors relay.TradeEvent to avoid circular imports.
type TradeEvent struct {
	Time     time.Time
	Market   string
	Action   string
	Success  bool
	Error    string
	SignalID string
}

// DiscordWebhook sends trade notifications to a Discord channel.
type DiscordWebhook struct {
	url        string
	client     *http.Client
	mu         sync.Mutex
	lastSent   time.Time
	minInterval time.Duration
}

// NewDiscordWebhook creates a webhook notifier. Set url to "" to disable.
func NewDiscordWebhook(url string) *DiscordWebhook {
	return &DiscordWebhook{
		url: url,
		client: &http.Client{Timeout: 5 * time.Second},
		minInterval: 2 * time.Second, // Discord rate limit safety
	}
}

// RecordTrade sends a color-coded embed to Discord.
// Non-blocking: fires in a goroutine so it never delays execution.
func (d *DiscordWebhook) RecordTrade(e TradeEvent) {
	if d.url == "" {
		return
	}
	go d.send(e)
}

func (d *DiscordWebhook) send(e TradeEvent) {
	d.mu.Lock()
	if time.Since(d.lastSent) < d.minInterval {
		d.mu.Unlock()
		return // rate limited, drop
	}
	d.lastSent = time.Now()
	d.mu.Unlock()

	color := 0x44FF44 // green
	title := fmt.Sprintf("%s %s", e.Market, e.Action)
	if !e.Success {
		color = 0xFF4444 // red
		title += " (FAILED)"
	}

	fields := []embedField{
		{Name: "Market", Value: e.Market, Inline: true},
		{Name: "Action", Value: e.Action, Inline: true},
	}
	if e.SignalID != "" {
		fields = append(fields, embedField{Name: "Signal", Value: e.SignalID, Inline: true})
	}
	if e.Error != "" {
		fields = append(fields, embedField{Name: "Error", Value: truncate(e.Error, 200), Inline: false})
	}

	payload := webhookPayload{
		Embeds: []embed{{
			Title:     title,
			Color:     color,
			Fields:    fields,
			Timestamp: e.Time.UTC().Format(time.RFC3339),
		}},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("discord webhook: marshal error", "error", err)
		return
	}

	resp, err := d.client.Post(d.url, "application/json", bytes.NewReader(data))
	if err != nil {
		slog.Debug("discord webhook: send error", "error", err)
		return
	}
	resp.Body.Close()
}

// SendAlert sends a custom alert (e.g. circuit breaker, risk limit).
func (d *DiscordWebhook) SendAlert(title, message string, color int) {
	if d.url == "" {
		return
	}
	go func() {
		payload := webhookPayload{
			Embeds: []embed{{
				Title:       title,
				Description: message,
				Color:       color,
				Timestamp:   time.Now().UTC().Format(time.RFC3339),
			}},
		}
		data, _ := json.Marshal(payload)
		resp, err := d.client.Post(d.url, "application/json", bytes.NewReader(data))
		if err != nil {
			return
		}
		resp.Body.Close()
	}()
}

type webhookPayload struct {
	Embeds []embed `json:"embeds"`
}

type embed struct {
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color"`
	Fields      []embedField `json:"fields,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
}

type embedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
