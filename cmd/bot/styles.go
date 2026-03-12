package main

import "github.com/charmbracelet/lipgloss"

var (
	colorBrand = lipgloss.Color("#5EEAD4")

	styleHeader = lipgloss.NewStyle().
			Foreground(colorBrand).
			Bold(true)

	styleHeaderDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	styleHeaderLabel = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9CA3AF")).
				Width(12)

	styleHeaderValue = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E5E7EB"))

	styleSeparator = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#374151"))

	styleStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D5DB"))

	styleKeybindHint = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280"))

	styleKeybindKey = lipgloss.NewStyle().
			Foreground(colorBrand)

	styleViewTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBrand).
			MarginBottom(1)

	styleLogTime = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	styleLogInfo = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))

	styleLogWarn = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FBBF24"))

	styleLogError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)

	styleLogDebug = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	styleLogAttr = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	styleGreen = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#34D399"))

	styleRed = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F87171"))

	styleMainnet = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#34D399")).
			Bold(true)

	styleTestnet = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FBBF24")).
			Bold(true)

	styleConnected    = lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))
	styleDisconnected = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
)
