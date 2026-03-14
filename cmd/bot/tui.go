package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/relay"
)

type viewMode int

const (
	viewLogs viewMode = iota
	viewTrades
	viewPositions
	viewSettings
	viewUpdateConfirm
	viewUpdateProgress
)

const maxLogLines = 1000

type accountUpdateMsg struct {
	state     *hyperliquid.ClearinghouseState
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

type tuiModel struct {
	width, height int

	instanceName string
	walletAddr   string
	network      string
	callerName   string
	targetWallet string
	paused       bool

	accountState *hyperliquid.ClearinghouseState
	connected    bool

	activeView viewMode
	logView    viewport.Model
	logLines   []string
	tradeStore *TradeStore
	clock      time.Time

	settings *SettingsSnapshot

	// Log handler (for flushing buffered pre-TUI logs)
	logHandler *TUILogHandler

	updateAvailable bool
	latestVersion   string
	updateStatus    string // "downloading", "replacing", "restarting", "done", "error"
	updateError     string
	updateCompleted bool

	quitting bool
	quitFunc func()
}

func newTUIModel(
	instanceName, walletAddr, network string,
	state *hyperliquid.ClearinghouseState,
	connected bool,
	tradeStore *TradeStore,
	settings *SettingsSnapshot,
	logHandler *TUILogHandler,
	quitFunc func(),
) tuiModel {
	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true

	return tuiModel{
		instanceName: instanceName,
		walletAddr:   walletAddr,
		network:      network,
		accountState: state,
		connected:    connected,
		logView:      vp,
		tradeStore:   tradeStore,
		settings:     settings,
		logHandler:   logHandler,
		clock:        time.Now(),
		quitFunc:     quitFunc,
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
		body := renderSettings(m.settings, m.connected)
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
		equity := formatUSD(m.accountState.MarginSummary.AccountValue)
		margin := formatUSD(m.accountState.MarginSummary.TotalMarginUsed)
		marginPct := calcMarginPct(m.accountState.MarginSummary.TotalMarginUsed, m.accountState.MarginSummary.AccountValue)
		free := formatUSD(m.accountState.Withdrawable)
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

	sb.WriteString(sep)
	sb.WriteString("\n")

	return sb.String()
}

func (m tuiModel) renderFooter() string {
	sep := styleSeparator.Render(strings.Repeat("─", max(0, m.width)))

	var statusParts []string
	if m.accountState != nil {
		statusParts = append(statusParts, formatUSD(m.accountState.MarginSummary.AccountValue))
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
	case viewLogs:
		hints = " " + styleKeybindKey.Render("[t]") + styleKeybindHint.Render("rades  ") +
			styleKeybindKey.Render("[p]") + styleKeybindHint.Render("ositions  ") +
			styleKeybindKey.Render("[s]") + styleKeybindHint.Render("ettings  ")
		if m.updateAvailable {
			hints += styleKeybindKey.Render("[u]") + styleKeybindHint.Render("pdate  ")
		}
		hints += styleKeybindKey.Render("[q]") + styleKeybindHint.Render("uit (stop bot)")
	default: // trades, positions, settings
		hints = " " + styleKeybindKey.Render("[esc]") + styleKeybindHint.Render(" back  ")
		if m.updateAvailable {
			hints += styleKeybindKey.Render("[u]") + styleKeybindHint.Render("pdate  ")
		}
		hints += styleKeybindKey.Render("[q]") + styleKeybindHint.Render("uit (stop bot)")
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
