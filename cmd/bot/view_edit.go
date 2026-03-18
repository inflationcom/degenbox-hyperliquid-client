package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/config"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/keystore"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/relay"
)

type editStep int

const (
	editStepNone editStep = iota
	// API key
	editStepAPIKeyInput
	editStepAPIKeyRegistering
	editStepAPIKeyDone
	// Encrypt
	editStepEncryptPass1
	editStepEncryptPass2
	editStepEncrypting
	editStepEncryptDone
	// Risk limits
	editStepRiskLeverage
	editStepRiskOrderSize
	editStepRiskDone
	// Ticker
	editStepTickerToggle
	// Delete
	editStepDeleteConfirm
	editStepDeleting
	editStepDeleteDone
)

type editState struct {
	step    editStep
	input   textinput.Model
	err     string
	success string

	// intermediate values
	passphrase1 string
	newLeverage int
}

// async result messages
type registrationDoneMsg struct{ apiKey string }
type registrationErrMsg struct{ err error }
type encryptDoneMsg struct{}
type encryptErrMsg struct{ err error }
type deleteDoneMsg struct{}
type deleteErrMsg struct{ err error }

func newTextInput(placeholder string, masked bool) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 256
	if masked {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}
	return ti
}

func initAPIKeyEdit() editState {
	return editState{
		step:  editStepAPIKeyInput,
		input: newTextInput("rt_...", false),
	}
}

func initEncryptEdit() editState {
	// Check if private key exists in .env
	pk := readPrivateKeyFromEnvFile()
	if pk == "" {
		return editState{
			step: editStepEncryptDone,
			err:  "No private key found in .env file",
		}
	}
	if keystore.Exists(defaultKeystorePath) {
		return editState{
			step:  editStepEncryptPass1,
			input: newTextInput("min 8 characters", true),
			err:   "Keystore exists — new passphrase will overwrite it",
		}
	}
	return editState{
		step:  editStepEncryptPass1,
		input: newTextInput("min 8 characters", true),
	}
}

func initRiskEdit(currentLeverage int) editState {
	ti := newTextInput(fmt.Sprintf("current: %d", currentLeverage), false)
	return editState{
		step:  editStepRiskLeverage,
		input: ti,
	}
}

func initDeleteEdit() editState {
	return editState{
		step:  editStepDeleteConfirm,
		input: newTextInput("type YES to confirm", false),
	}
}

// ── Render ──

func renderEdit(e *editState, settings *SettingsSnapshot, tickerEnabled map[string]bool) string {
	var sb strings.Builder
	sb.WriteString("\n")

	switch e.step {
	// ── API Key ──
	case editStepAPIKeyInput:
		sb.WriteString(styleHeaderValue.Render("  Update API Key") + "\n\n")
		sb.WriteString(styleHeaderDim.Render("  Paste your registration token from the dashboard.") + "\n")
		sb.WriteString(styleHeaderDim.Render("  (Wallets > Add Wallet > Generate)") + "\n\n")
		sb.WriteString("  Token: " + e.input.View() + "\n")
	case editStepAPIKeyRegistering:
		sb.WriteString(styleHeaderValue.Render("  Update API Key") + "\n\n")
		sb.WriteString(styleLogWarn.Render("  Registering...") + "\n")
	case editStepAPIKeyDone:
		sb.WriteString(styleHeaderValue.Render("  Update API Key") + "\n\n")

	// ── Encrypt ──
	case editStepEncryptPass1:
		sb.WriteString(styleHeaderValue.Render("  Encrypt Wallet") + "\n\n")
		sb.WriteString(styleHeaderDim.Render("  Create a passphrase to encrypt your private key.") + "\n")
		sb.WriteString(styleHeaderDim.Render("  The key will be stored in .keystore.json (AES-256-GCM).") + "\n\n")
		sb.WriteString("  Passphrase: " + e.input.View() + "\n")
	case editStepEncryptPass2:
		sb.WriteString(styleHeaderValue.Render("  Encrypt Wallet") + "\n\n")
		sb.WriteString("  Confirm:    " + e.input.View() + "\n")
	case editStepEncrypting:
		sb.WriteString(styleHeaderValue.Render("  Encrypt Wallet") + "\n\n")
		sb.WriteString(styleLogWarn.Render("  Encrypting (this takes a few seconds)...") + "\n")
	case editStepEncryptDone:
		sb.WriteString(styleHeaderValue.Render("  Encrypt Wallet") + "\n\n")

	// ── Risk Limits ──
	case editStepRiskLeverage:
		sb.WriteString(styleHeaderValue.Render("  Edit Risk Limits") + "\n\n")
		sb.WriteString("  Max leverage: " + e.input.View() + "\n")
	case editStepRiskOrderSize:
		sb.WriteString(styleHeaderValue.Render("  Edit Risk Limits") + "\n\n")
		sb.WriteString(styleGreen.Render(fmt.Sprintf("  Max leverage: %d", e.newLeverage)) + "  " + styleGreen.Render("✓") + "\n")
		sb.WriteString("  Max order USD: " + e.input.View() + "\n")
	case editStepRiskDone:
		sb.WriteString(styleHeaderValue.Render("  Edit Risk Limits") + "\n\n")

	// ── Ticker ──
	case editStepTickerToggle:
		sb.WriteString(styleHeaderValue.Render("  Ticker Assets") + "\n\n")
		for i, ta := range tickerAssetList {
			check := styleHeaderDim.Render("  [ ] ")
			if tickerEnabled[ta.Display] {
				check = styleGreen.Render("  [✓] ")
			}
			sb.WriteString(check + styleHeaderValue.Render(fmt.Sprintf("%d. %s", i+1, ta.Display)) + "\n")
		}
		sb.WriteString("\n" + styleHeaderDim.Render(fmt.Sprintf("  Press 1-%d to toggle, esc to save", len(tickerAssetList))) + "\n")

	// ── Delete ──
	case editStepDeleteConfirm:
		sb.WriteString(styleRed.Render("  Delete Wallet") + "\n\n")
		sb.WriteString(styleHeaderValue.Render("  This will remove your wallet from the dashboard") + "\n")
		sb.WriteString(styleHeaderValue.Render("  and disconnect the bot permanently.") + "\n\n")
		sb.WriteString("  " + e.input.View() + "\n")
	case editStepDeleting:
		sb.WriteString(styleRed.Render("  Delete Wallet") + "\n\n")
		sb.WriteString(styleLogWarn.Render("  Deleting...") + "\n")
	case editStepDeleteDone:
		sb.WriteString(styleRed.Render("  Delete Wallet") + "\n\n")
	}

	// Show error or success
	if e.err != "" {
		sb.WriteString("\n  " + styleRed.Render(e.err) + "\n")
	}
	if e.success != "" {
		sb.WriteString("\n  " + styleGreen.Render(e.success) + "\n")
	}

	return sb.String()
}

// ── Async commands ──

func doRegister(serverWSURL, token, name, walletAddr, mainWalletAddr, network string) tea.Cmd {
	return func() tea.Msg {
		httpURL := wsToHTTP(serverWSURL)
		result, err := relay.Register(httpURL, token, name, walletAddr, mainWalletAddr, network)
		if err != nil {
			return registrationErrMsg{err: err}
		}
		return registrationDoneMsg{apiKey: result.APIKey}
	}
}

func doEncrypt(passphrase string) tea.Cmd {
	return func() tea.Msg {
		pk := readPrivateKeyFromEnvFile()
		if pk == "" {
			return encryptErrMsg{err: fmt.Errorf("no private key found in .env")}
		}
		pk = strings.TrimPrefix(pk, "0x")
		if err := keystore.Encrypt(pk, []byte(passphrase), defaultKeystorePath); err != nil {
			return encryptErrMsg{err: err}
		}
		return encryptDoneMsg{}
	}
}

func doDeleteSelf(relayClient *relay.Client) tea.Cmd {
	return func() tea.Msg {
		if err := relayClient.DeleteSelf(); err != nil {
			return deleteErrMsg{err: err}
		}
		return deleteDoneMsg{}
	}
}

// ── Helpers ──

func wsToHTTP(wsURL string) string {
	u := strings.TrimSuffix(wsURL, "/ws")
	u = strings.Replace(u, "wss://", "https://", 1)
	u = strings.Replace(u, "ws://", "http://", 1)
	return u
}

func readPrivateKeyFromEnvFile() string {
	data, err := os.ReadFile(".env")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "HL_PRIVATE_KEY=") {
			val := strings.TrimPrefix(line, "HL_PRIVATE_KEY=")
			return strings.Trim(val, "\"'")
		}
	}
	return ""
}

func saveAPIKeyToEnv(apiKey string) error {
	envMap := readEnvFile(".env")
	envMap["HL_RELAY_API_KEY"] = apiKey

	var lines []string
	for k, v := range envMap {
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}
	return os.WriteFile(".env", []byte(strings.Join(lines, "\n")+"\n"), 0600)
}

func saveRiskLimitsToConfig(configPath string, leverage int, orderSize float64) error {
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		return err
	}
	cfg.RiskLimits.MaxLeverage = leverage
	cfg.RiskLimits.MaxOrderSizeUSD = orderSize
	return cfg.SaveToFile(configPath)
}

func saveTickerAssetsToConfig(configPath string, enabled map[string]bool) error {
	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		return err
	}
	var assets []string
	for _, ta := range tickerAssetList {
		if enabled[ta.Display] {
			assets = append(assets, ta.Display)
		}
	}
	cfg.TickerAssets = assets
	return cfg.SaveToFile(configPath)
}

// ── Ticker asset mapping ──

type tickerAsset struct {
	Display string // shown in TUI
	Market  string // HL market name for allMids
}

var tickerAssetList = []tickerAsset{
	{"BTC", "BTC"},
	{"ETH", "ETH"},
	{"SOL", "SOL"},
	{"HYPE", "HYPE"},
	{"GOLD", "xyz:GOLD"},
	{"SILVER", "xyz:SILVER"},
	{"OIL", "xyz:CL"},
	{"XYZ100", "xyz:XYZ100"},
}

var defaultTickerAssets = []string{"BTC", "ETH", "SOL"}

func buildTickerEnabled(assets []string) map[string]bool {
	m := make(map[string]bool)
	if len(assets) == 0 {
		assets = defaultTickerAssets
	}
	for _, a := range assets {
		m[a] = true
	}
	return m
}

func formatTickerPrice(price float64) string {
	if price >= 10000 {
		return fmt.Sprintf("$%.0f", price)
	}
	if price >= 100 {
		return fmt.Sprintf("$%.1f", price)
	}
	if price >= 1 {
		return fmt.Sprintf("$%.2f", price)
	}
	return fmt.Sprintf("$%.4f", price)
}

func renderTickerLine(prices map[string]float64, enabled map[string]bool) string {
	var parts []string
	for _, ta := range tickerAssetList {
		if !enabled[ta.Display] {
			continue
		}
		price, ok := prices[ta.Market]
		if !ok || price == 0 {
			continue
		}
		parts = append(parts, styleHeaderDim.Render(ta.Display+" ")+styleHeaderValue.Render(formatTickerPrice(price)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, "  ")
}

// parseAllMids converts the string map from HL API to float64 map
func parseAllMids(mids map[string]string) map[string]float64 {
	result := make(map[string]float64, len(mids))
	for k, v := range mids {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			result[k] = f
		}
	}
	return result
}
