package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/config"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/relay"
)

type viewMode int

const (
	viewLogs viewMode = iota
	viewTrades
	viewPositions
	viewSettings
	viewEdit
	viewUpdateConfirm
	viewUpdateProgress
)

const maxLogLines = 1000

type accountUpdateMsg struct {
	state     *hyperliquid.ClearinghouseState
	spotUSDC  float64
	connected bool
}

type nameUpdateMsg string
type pauseUpdateMsg bool

type versionInfoMsg relay.VersionInfoMsg

type authInfoMsg relay.AuthInfoMsg

type assignmentUpdateMsg relay.AssignmentUpdateMsg

type clockTickMsg time.Time

type updateDoneMsg struct{}
type updateErrorMsg struct{ err error }
type tickerUpdateMsg map[string]float64

type tuiModel struct {
	width, height int

	instanceName string
	walletAddr   string
	network      string
	callerName   string
	targetWallet string
	paused       bool

	accountState *hyperliquid.ClearinghouseState
	spotUSDC     float64
	connected    bool

	activeView viewMode
	logView    viewport.Model
	logLines   []string
	tradeStore *TradeStore
	clock      time.Time

	settings *SettingsSnapshot

	// Log handler (for flushing buffered pre-TUI logs)
	logHandler *TUILogHandler

	// Edit state
	edit           editState
	configPath     string
	relayClient    *relay.Client
	relayServerURL string
	isAgentMode    bool
	mainWalletAddr string
	validator      *relay.RiskValidator

	// Ticker
	tickerPrices  map[string]float64
	tickerEnabled map[string]bool

	updateAvailable bool
	latestVersion   string
	updateStatus    string // "downloading", "replacing", "restarting", "done", "error"
	updateError     string
	updateCompleted bool

	quitting bool
	quitFunc func()
}

type tuiConfig struct {
	instanceName   string
	walletAddr     string
	network        string
	state          *hyperliquid.ClearinghouseState
	spotUSDC       float64
	connected      bool
	tradeStore     *TradeStore
	settings       *SettingsSnapshot
	logHandler     *TUILogHandler
	quitFunc       func()
	configPath     string
	relayClient    *relay.Client
	relayServerURL string
	isAgentMode    bool
	mainWalletAddr string
	validator      *relay.RiskValidator
	tickerAssets   []string
}

func newTUIModel(c tuiConfig) tuiModel {
	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true

	return tuiModel{
		instanceName:   c.instanceName,
		walletAddr:     c.walletAddr,
		network:        c.network,
		accountState:   c.state,
		spotUSDC:       c.spotUSDC,
		connected:      c.connected,
		logView:        vp,
		tradeStore:     c.tradeStore,
		settings:       c.settings,
		logHandler:     c.logHandler,
		clock:          time.Now(),
		quitFunc:       c.quitFunc,
		configPath:     c.configPath,
		relayClient:    c.relayClient,
		relayServerURL: c.relayServerURL,
		isAgentMode:    c.isAgentMode,
		mainWalletAddr: c.mainWalletAddr,
		validator:      c.validator,
		tickerPrices:   make(map[string]float64),
		tickerEnabled:  buildTickerEnabled(c.tickerAssets),
	}
}

func (m tuiModel) Init() tea.Cmd {
	cmds := []tea.Cmd{tickEverySecond()}
	if m.logHandler != nil {
		cmds = append(cmds, m.logHandler.FlushCmd())
	}
	return tea.Batch(cmds...)
}

func tickEverySecond() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return clockTickMsg(t)
	})
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch m.activeView {
		case viewUpdateConfirm:
			switch msg.String() {
			case "y":
				m.activeView = viewUpdateProgress
				m.updateStatus = "downloading"
				return m, m.startUpdate()
			case "n", "esc":
				m.activeView = viewLogs
			case "q", "ctrl+c":
				m.quitting = true
				if m.quitFunc != nil {
					m.quitFunc()
				}
				return m, tea.Quit
			}
		case viewUpdateProgress:
			switch msg.String() {
			case "esc":
				if m.updateStatus == "error" {
					m.activeView = viewLogs
				}
			case "q", "ctrl+c":
				if m.updateStatus == "error" {
					m.quitting = true
					if m.quitFunc != nil {
						m.quitFunc()
					}
					return m, tea.Quit
				}
			}
		case viewEdit:
			return m.handleEditKey(msg)
		case viewSettings:
			switch msg.String() {
			case "1":
				m.edit = initAPIKeyEdit()
				m.activeView = viewEdit
			case "2":
				m.edit = initEncryptEdit()
				m.activeView = viewEdit
			case "3":
				lev := 20
				if m.settings != nil {
					lev = m.settings.MaxLeverage
				}
				m.edit = initRiskEdit(lev)
				m.activeView = viewEdit
			case "4":
				m.edit = editState{step: editStepTickerToggle}
				m.activeView = viewEdit
			case "5":
				// Toggle pause
				if m.relayClient != nil {
					newPaused := !m.paused
					if err := m.relayClient.SendPause(newPaused); err == nil {
						m.paused = newPaused
					}
				}
			case "6":
				m.edit = initDeleteEdit()
				m.activeView = viewEdit
			case "q", "ctrl+c":
				m.quitting = true
				if m.quitFunc != nil {
					m.quitFunc()
				}
				return m, tea.Quit
			case "u":
				if m.updateAvailable {
					m.activeView = viewUpdateConfirm
				}
			case "esc", "enter":
				m.activeView = viewLogs
			}
		default:
			switch msg.String() {
			case "q", "ctrl+c":
				m.quitting = true
				if m.quitFunc != nil {
					m.quitFunc()
				}
				return m, tea.Quit
			case "t":
				m.activeView = viewTrades
			case "p":
				m.activeView = viewPositions
			case "s":
				m.activeView = viewSettings
			case "u":
				if m.updateAvailable {
					m.activeView = viewUpdateConfirm
				}
			case " ":
				// Space toggles pause from any view
				if m.relayClient != nil {
					newPaused := !m.paused
					if err := m.relayClient.SendPause(newPaused); err == nil {
						m.paused = newPaused
					}
				}
			case "esc", "enter":
				m.activeView = viewLogs
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentHeight := m.contentHeight()
		m.logView.Width = msg.Width
		m.logView.Height = contentHeight

	case flushLogsMsg:
		m.logLines = append(m.logLines, []string(msg)...)
		if len(m.logLines) > maxLogLines {
			m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
		}
		m.logView.SetContent(strings.Join(m.logLines, "\n"))
		m.logView.GotoBottom()

	case logMsg:
		m.logLines = append(m.logLines, string(msg))
		if len(m.logLines) > maxLogLines {
			m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
		}
		m.logView.SetContent(strings.Join(m.logLines, "\n"))
		m.logView.GotoBottom()

	case accountUpdateMsg:
		m.accountState = msg.state
		m.spotUSDC = msg.spotUSDC
		m.connected = msg.connected

	case nameUpdateMsg:
		m.instanceName = string(msg)

	case pauseUpdateMsg:
		m.paused = bool(msg)

	case authInfoMsg:
		if len(msg.Callers) > 0 {
			m.callerName = msg.Callers[0]
		}
		m.targetWallet = msg.CopytradeTarget

	case assignmentUpdateMsg:
		if msg.CallerName != nil {
			m.callerName = *msg.CallerName
		} else {
			m.callerName = ""
		}
		if msg.CopytradeTarget != nil {
			m.targetWallet = *msg.CopytradeTarget
		} else {
			m.targetWallet = ""
		}

	case versionInfoMsg:
		m.updateAvailable = msg.UpdateAvailable
		m.latestVersion = msg.LatestVersion

	case tickerUpdateMsg:
		m.tickerPrices = map[string]float64(msg)

	case registrationDoneMsg:
		if m.relayClient != nil {
			m.relayClient.UpdateAPIKey(msg.apiKey)
		}
		_ = saveAPIKeyToEnv(msg.apiKey)
		m.edit.step = editStepAPIKeyDone
		m.edit.success = "API key updated — reconnecting..."
		m.edit.err = ""

	case registrationErrMsg:
		m.edit.step = editStepAPIKeyInput
		m.edit.err = msg.err.Error()
		m.edit.input = newTextInput("rt_...", false)

	case encryptDoneMsg:
		m.edit.step = editStepEncryptDone
		m.edit.success = "Keystore saved to .keystore.json — you can delete .env"
		m.edit.err = ""

	case encryptErrMsg:
		m.edit.step = editStepEncryptDone
		m.edit.err = msg.err.Error()

	case deleteDoneMsg:
		m.edit.step = editStepDeleteDone
		m.edit.success = "Wallet deleted. Bot will exit."
		m.edit.err = ""
		m.quitting = true
		return m, tea.Quit

	case deleteErrMsg:
		m.edit.step = editStepDeleteDone
		m.edit.err = msg.err.Error()

	case updateDoneMsg:
		m.updateStatus = "restarting"
		m.updateCompleted = true
		m.quitting = true
		return m, tea.Quit

	case updateErrorMsg:
		m.updateStatus = "error"
		m.updateError = msg.err.Error()

	case clockTickMsg:
		m.clock = time.Time(msg)
		return m, tickEverySecond()
	}

	// Forward to viewport for scroll handling
	if m.activeView == viewLogs {
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "Starting..."
	}

	var sb strings.Builder
	header := m.renderHeader()
	sb.WriteString(header)

	contentHeight := m.contentHeight()
	var content string
	switch m.activeView {
	case viewTrades:
		title := styleViewTitle.Render("  Recent Trades")
		body := renderTrades(m.tradeStore, m.width)
		content = title + "\n" + body
	case viewPositions:
		title := styleViewTitle.Render("  Positions")
		body := m.renderPositionsView()
		content = title + "\n" + body
	case viewSettings:
		title := styleViewTitle.Render("  Settings")
		body := renderSettings(m.settings, m.connected, m.paused)
		content = title + "\n" + body
	case viewEdit:
		title := styleViewTitle.Render("  Settings")
		body := renderEdit(&m.edit, m.settings, m.tickerEnabled)
		content = title + "\n" + body
	case viewUpdateConfirm:
		title := styleViewTitle.Render("  Update")
		body := renderUpdateConfirm(m.latestVersion, version)
		content = title + "\n" + body
	case viewUpdateProgress:
		title := styleViewTitle.Render("  Update")
		body := renderUpdateProgress(m.updateStatus, m.updateError)
		content = title + "\n" + body
	default:
		content = m.logView.View()
	}

	contentLines := strings.Split(content, "\n")
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, "")
	}
	sb.WriteString(strings.Join(contentLines, "\n"))
	sb.WriteString("\n")

	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m tuiModel) renderHeader() string {
	var sb strings.Builder
	sb.WriteString(styleHeader.Render(logo))
	sb.WriteString("\n\n")

	sb.WriteString("  ")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(m.instanceName))
	sb.WriteString("  ")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(version))
	if m.paused {
		sb.WriteString("  ")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF6666")).Render("PAUSED"))
	}
	sb.WriteString("\n")

	if m.updateAvailable && m.latestVersion != "" {
		sb.WriteString("  ")
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00")).Render(
			fmt.Sprintf("Update available: %s → %s  — press [u] to update", version, m.latestVersion)))
		sb.WriteString("\n")
	}

	sep := styleSeparator.Render("  " + strings.Repeat("─", max(0, min(m.width-4, 50))))
	sb.WriteString(sep)
	sb.WriteString("\n")

	netLabel := strings.ToUpper(m.network)
	netStyled := styleTestnet.Render(netLabel)
	if m.network == "mainnet" {
		netStyled = styleMainnet.Render(netLabel)
	}

	pad := func(s string, w int) string {
		if len(s) >= w {
			return s
		}
		return s + strings.Repeat(" ", w-len(s))
	}

	sb.WriteString(styleHeaderLabel.Render("  Wallet") + styleHeaderValue.Render(pad(shortenAddr(m.walletAddr), 16)) + styleHeaderLabel.Render("Network") + "  " + netStyled)
	sb.WriteString("\n")

	if m.accountState != nil {
		perpEquity, _ := strconv.ParseFloat(m.accountState.MarginSummary.AccountValue, 64)
		totalEquity := perpEquity + m.spotUSDC
		equity := formatUSDf(totalEquity)
		margin := formatUSD(m.accountState.MarginSummary.TotalMarginUsed)
		marginPct := calcMarginPct(m.accountState.MarginSummary.TotalMarginUsed, m.accountState.MarginSummary.AccountValue)
		withdrawable, _ := strconv.ParseFloat(m.accountState.Withdrawable, 64)
		free := formatUSDf(withdrawable + m.spotUSDC)
		positions := countPositions(m.accountState.AssetPositions)

		sb.WriteString(styleHeaderLabel.Render("  Equity") + styleHeaderValue.Render(pad(equity, 16)) + styleHeaderLabel.Render("Margin") + "  " + styleHeaderValue.Render(margin+" ("+marginPct+")"))
		sb.WriteString("\n")
		sb.WriteString(styleHeaderLabel.Render("  Free") + styleHeaderValue.Render(pad(free, 16)) + styleHeaderLabel.Render("Positions") + "  " + styleHeaderValue.Render(fmt.Sprintf("%d", positions)))
		sb.WriteString("\n")
	}

	if m.callerName != "" {
		sb.WriteString(styleHeaderLabel.Render("  Source") + styleHeaderValue.Render(m.callerName))
		sb.WriteString("\n")
	} else if m.targetWallet != "" {
		sb.WriteString(styleHeaderLabel.Render("  Source") + styleHeaderValue.Render(shortenAddr(m.targetWallet)+" (copytrade)"))
		sb.WriteString("\n")
	} else if m.connected {
		sb.WriteString(styleHeaderLabel.Render("  Source") + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("no signal source linked"))
		sb.WriteString("\n")
	}

	// Ticker line
	if ticker := renderTickerLine(m.tickerPrices, m.tickerEnabled); ticker != "" {
		sb.WriteString(ticker)
		sb.WriteString("\n")
	}

	sb.WriteString(sep)
	sb.WriteString("\n")

	return sb.String()
}

func (m tuiModel) renderFooter() string {
	sep := styleSeparator.Render(strings.Repeat("─", max(0, m.width)))

	var statusParts []string
	if m.accountState != nil {
		perpEquity, _ := strconv.ParseFloat(m.accountState.MarginSummary.AccountValue, 64)
		statusParts = append(statusParts, formatUSDf(perpEquity+m.spotUSDC))
		marginPct := calcMarginPct(m.accountState.MarginSummary.TotalMarginUsed, m.accountState.MarginSummary.AccountValue)
		statusParts = append(statusParts, marginPct)
		statusParts = append(statusParts, fmt.Sprintf("%d pos", countPositions(m.accountState.AssetPositions)))
	}

	indicator := styleConnected.Render("●")
	if !m.connected {
		indicator = styleDisconnected.Render("○")
	}
	statusParts = append(statusParts, indicator)
	statusParts = append(statusParts, m.clock.Format(time.TimeOnly))

	status := " " + styleStatusBar.Render(strings.Join(statusParts, " │ "))

	var hints string
	switch m.activeView {
	case viewUpdateConfirm:
		hints = " " + styleKeybindKey.Render("[y]") + styleKeybindHint.Render("es  ") +
			styleKeybindKey.Render("[n]") + styleKeybindHint.Render("o")
	case viewUpdateProgress:
		if m.updateStatus == "error" {
			hints = " " + styleKeybindKey.Render("[esc]") + styleKeybindHint.Render(" back")
		} else {
			hints = " " + styleKeybindHint.Render("updating...")
		}
	case viewEdit:
		switch m.edit.step {
		case editStepAPIKeyDone, editStepEncryptDone, editStepRiskDone, editStepDeleteDone:
			hints = " " + styleKeybindKey.Render("[esc]") + styleKeybindHint.Render(" back")
		case editStepAPIKeyRegistering, editStepEncrypting, editStepDeleting:
			hints = " " + styleKeybindHint.Render("please wait...")
		case editStepTickerToggle:
			hints = " " + styleKeybindKey.Render(fmt.Sprintf("[1-%d]", len(tickerAssetList))) + styleKeybindHint.Render(" toggle  ") +
				styleKeybindKey.Render("[esc]") + styleKeybindHint.Render(" save & back")
		default:
			hints = " " + styleKeybindKey.Render("[enter]") + styleKeybindHint.Render(" submit  ") +
				styleKeybindKey.Render("[esc]") + styleKeybindHint.Render(" cancel")
		}
	case viewLogs:
		hints = " " + styleKeybindKey.Render("[t]") + styleKeybindHint.Render("rades  ") +
			styleKeybindKey.Render("[p]") + styleKeybindHint.Render("ositions  ") +
			styleKeybindKey.Render("[s]") + styleKeybindHint.Render("ettings  ")
		if m.updateAvailable {
			hints += styleKeybindKey.Render("[u]") + styleKeybindHint.Render("pdate  ")
		}
		hints += styleKeybindKey.Render("[space]") + styleKeybindHint.Render(" pause  ")
		hints += styleKeybindKey.Render("[q]") + styleKeybindHint.Render("uit")
	default: // trades, positions, settings
		hints = " " + styleKeybindKey.Render("[esc]") + styleKeybindHint.Render(" back  ")
		if m.updateAvailable {
			hints += styleKeybindKey.Render("[u]") + styleKeybindHint.Render("pdate  ")
		}
		hints += styleKeybindKey.Render("[q]") + styleKeybindHint.Render("uit")
	}

	return sep + "\n" + status + "\n\n" + hints
}

func (m tuiModel) renderPositionsView() string {
	if m.accountState == nil {
		return styleHeaderDim.Render("  No account data")
	}
	return renderPositions(m.accountState.AssetPositions, m.width)
}

func (m tuiModel) headerLines() int {
	lines := 8 // logo (6: leading newline + 5 art) + blank + name
	if m.updateAvailable {
		lines += 1
	}
	lines += 2 // separator + wallet/network
	if m.accountState != nil {
		lines += 2 // equity/margin + free/positions
	}
	if m.callerName != "" {
		lines += 1
	}
	if m.targetWallet != "" {
		lines += 1
	}
	if renderTickerLine(m.tickerPrices, m.tickerEnabled) != "" {
		lines += 1
	}
	lines += 1 // bottom separator
	return lines
}

const footerLines = 4 // separator + status + blank + keybinds

func (m tuiModel) contentHeight() int {
	h := m.height - m.headerLines() - footerLines
	if h < 1 {
		h = 1
	}
	return h
}

func (m tuiModel) startUpdate() tea.Cmd {
	return func() tea.Msg {
		if err := performUpdate(); err != nil {
			return updateErrorMsg{err: err}
		}
		return updateDoneMsg{}
	}
}

func (m tuiModel) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.edit.step {

	// ── Done states: esc goes back ──
	case editStepAPIKeyDone, editStepEncryptDone, editStepRiskDone, editStepDeleteDone:
		if msg.String() == "esc" || msg.String() == "enter" {
			m.activeView = viewSettings
		}
		return m, nil

	// ── Busy states: ignore input ──
	case editStepAPIKeyRegistering, editStepEncrypting, editStepDeleting:
		return m, nil

	// ── Ticker toggle ──
	case editStepTickerToggle:
		switch msg.String() {
		case "esc":
			_ = saveTickerAssetsToConfig(m.configPath, m.tickerEnabled)
			m.activeView = viewSettings
		case "1", "2", "3", "4", "5", "6", "7", "8":
			idx, _ := strconv.Atoi(msg.String())
			idx-- // 0-based
			if idx < len(tickerAssetList) {
				name := tickerAssetList[idx].Display
				m.tickerEnabled[name] = !m.tickerEnabled[name]
			}
		}
		return m, nil

	// ── Delete confirm ──
	case editStepDeleteConfirm:
		switch msg.String() {
		case "esc":
			m.activeView = viewSettings
			return m, nil
		case "enter":
			if strings.ToUpper(strings.TrimSpace(m.edit.input.Value())) == "YES" {
				m.edit.step = editStepDeleting
				return m, doDeleteSelf(m.relayClient)
			}
			m.edit.err = "Type YES to confirm"
			m.edit.input.Reset()
			return m, nil
		default:
			var cmd tea.Cmd
			m.edit.input, cmd = m.edit.input.Update(msg)
			return m, cmd
		}

	// ── API key input ──
	case editStepAPIKeyInput:
		switch msg.String() {
		case "esc":
			m.activeView = viewSettings
			return m, nil
		case "enter":
			token := strings.TrimSpace(m.edit.input.Value())
			if !strings.HasPrefix(token, "rt_") {
				m.edit.err = "Token must start with rt_"
				return m, nil
			}
			m.edit.step = editStepAPIKeyRegistering
			m.edit.err = ""
			name := ""
			if m.settings != nil {
				name = m.settings.ClientName
			}
			walletAddr := m.walletAddr
			mainWallet := m.mainWalletAddr
			return m, doRegister(m.relayServerURL, token, name, walletAddr, mainWallet, m.network)
		default:
			var cmd tea.Cmd
			m.edit.input, cmd = m.edit.input.Update(msg)
			return m, cmd
		}

	// ── Encrypt pass 1 ──
	case editStepEncryptPass1:
		switch msg.String() {
		case "esc":
			m.activeView = viewSettings
			return m, nil
		case "enter":
			pass := m.edit.input.Value()
			if len(pass) < 8 {
				m.edit.err = "Passphrase must be at least 8 characters"
				return m, nil
			}
			m.edit.passphrase1 = pass
			m.edit.step = editStepEncryptPass2
			m.edit.input = newTextInput("confirm passphrase", true)
			m.edit.err = ""
			return m, nil
		default:
			var cmd tea.Cmd
			m.edit.input, cmd = m.edit.input.Update(msg)
			return m, cmd
		}

	// ── Encrypt pass 2 ──
	case editStepEncryptPass2:
		switch msg.String() {
		case "esc":
			m.activeView = viewSettings
			m.edit.passphrase1 = ""
			return m, nil
		case "enter":
			if m.edit.input.Value() != m.edit.passphrase1 {
				m.edit.err = "Passphrases do not match"
				m.edit.input = newTextInput("confirm passphrase", true)
				return m, nil
			}
			passphrase := m.edit.passphrase1
			m.edit.passphrase1 = ""
			m.edit.step = editStepEncrypting
			m.edit.err = ""
			return m, doEncrypt(passphrase)
		default:
			var cmd tea.Cmd
			m.edit.input, cmd = m.edit.input.Update(msg)
			return m, cmd
		}

	// ── Risk: leverage ──
	case editStepRiskLeverage:
		switch msg.String() {
		case "esc":
			m.activeView = viewSettings
			return m, nil
		case "enter":
			val := strings.TrimSpace(m.edit.input.Value())
			if val == "" && m.settings != nil {
				m.edit.newLeverage = m.settings.MaxLeverage
			} else {
				n, err := strconv.Atoi(val)
				if err != nil || n <= 0 {
					m.edit.err = "Enter a positive number"
					return m, nil
				}
				m.edit.newLeverage = n
			}
			m.edit.step = editStepRiskOrderSize
			m.edit.err = ""
			placeholder := "current: 5000"
			if m.settings != nil {
				placeholder = fmt.Sprintf("current: %.0f", m.settings.MaxOrderUSD)
			}
			m.edit.input = newTextInput(placeholder, false)
			return m, nil
		default:
			var cmd tea.Cmd
			m.edit.input, cmd = m.edit.input.Update(msg)
			return m, cmd
		}

	// ── Risk: order size ──
	case editStepRiskOrderSize:
		switch msg.String() {
		case "esc":
			m.activeView = viewSettings
			return m, nil
		case "enter":
			val := strings.TrimSpace(m.edit.input.Value())
			var orderSize float64
			if val == "" && m.settings != nil {
				orderSize = m.settings.MaxOrderUSD
			} else {
				f, err := strconv.ParseFloat(val, 64)
				if err != nil || f <= 0 {
					m.edit.err = "Enter a positive number"
					return m, nil
				}
				orderSize = f
			}
			// Save
			if err := saveRiskLimitsToConfig(m.configPath, m.edit.newLeverage, orderSize); err != nil {
				m.edit.step = editStepRiskDone
				m.edit.err = "Save failed: " + err.Error()
				return m, nil
			}
			// Hot-update
			if m.settings != nil {
				m.settings.MaxLeverage = m.edit.newLeverage
				m.settings.MaxOrderUSD = orderSize
			}
			if m.validator != nil {
				m.validator.UpdateLimits(config.RiskLimits{
					MaxLeverage:     m.edit.newLeverage,
					MaxOrderSizeUSD: orderSize,
					MaxPriceDevPct:  m.settings.MaxPriceDevPct,
					MaxPerMinute:    m.settings.MaxPerMinute,
				})
			}
			m.edit.step = editStepRiskDone
			m.edit.success = fmt.Sprintf("Saved: %d× leverage, $%.0f max order", m.edit.newLeverage, orderSize)
			return m, nil
		default:
			var cmd tea.Cmd
			m.edit.input, cmd = m.edit.input.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}
