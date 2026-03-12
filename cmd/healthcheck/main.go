package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Keep in sync with cmd/bot/heartbeat.go.
const defaultPath = "/tmp/degenbox-heartbeat.json"
const maxAge = 90 * time.Second

func main() {
	path := defaultPath
	if v := os.Getenv("HL_HEARTBEAT_PATH"); v != "" {
		path = v
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unhealthy: no heartbeat file\n")
		os.Exit(1)
	}

	var hb struct {
		Timestamp time.Time `json:"timestamp"`
		Connected bool      `json:"connected"`
		Version   string    `json:"version"`
	}
	if err := json.Unmarshal(data, &hb); err != nil {
		fmt.Fprintf(os.Stderr, "unhealthy: corrupt heartbeat\n")
		os.Exit(1)
	}

	age := time.Since(hb.Timestamp)
	if age > maxAge {
		fmt.Fprintf(os.Stderr, "unhealthy: heartbeat stale (%s ago)\n", age.Round(time.Second))
		os.Exit(1)
	}

	if !hb.Connected {
		fmt.Fprintf(os.Stderr, "degraded: relay disconnected (v%s)\n", hb.Version)
		os.Exit(0)
	}

	fmt.Printf("ok: connected (v%s)\n", hb.Version)
}
