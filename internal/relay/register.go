package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type RegistrationResult struct {
	ClientID        int    `json:"client_id"`
	Name            string `json:"name"`
	APIKey          string `json:"api_key"`
	Subscriptions   int    `json:"subscriptions"`
	ServerPublicKey string `json:"server_public_key"` // Ed25519 hex public key
	Message         string `json:"message"`
}

func Register(serverBaseURL, token, name, walletAddr, network string) (*RegistrationResult, error) {
	baseURL := strings.TrimRight(serverBaseURL, "/")

	body, err := json.Marshal(map[string]any{
		"name":           name,
		"wallet_address": walletAddr,
		"network":        network,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/api/clients/register", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 201 {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(respBody, &errResp)
		if errResp.Error != "" {
			return nil, fmt.Errorf("server error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result RegistrationResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result, nil
}
