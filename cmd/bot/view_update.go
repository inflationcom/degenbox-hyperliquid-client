package main

import (
	"fmt"
	"runtime"
	"strings"
)

func renderUpdateConfirm(latest, current string) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleHeaderLabel.Render("  Current version:  "))
	sb.WriteString(styleHeaderValue.Render(current))
	sb.WriteString("\n")
	sb.WriteString(styleHeaderLabel.Render("  New version:      "))
	sb.WriteString(styleGreen.Render(latest))
	sb.WriteString("\n\n")
	sb.WriteString(styleHeaderValue.Render("  The bot will download the update, replace the binary, and restart."))
	sb.WriteString("\n")
	sb.WriteString(styleHeaderValue.Render("  Your config and keys are not affected."))
	sb.WriteString("\n\n")
	sb.WriteString(styleKeybindKey.Render("  Press "))
	sb.WriteString(styleGreen.Render("y"))
	sb.WriteString(styleKeybindKey.Render(" to update, "))
	sb.WriteString(styleRed.Render("n"))
	sb.WriteString(styleKeybindKey.Render(" to cancel"))
	return sb.String()
}

func renderUpdateProgress(status, errMsg string) string {
	var sb strings.Builder
	sb.WriteString("\n")

	switch status {
	case "downloading":
		sb.WriteString(styleHeaderValue.Render("  Downloading update..."))
	case "replacing":
		sb.WriteString(styleHeaderValue.Render("  Replacing binary..."))
	case "restarting":
		sb.WriteString(styleGreen.Render("  Update complete! Restarting..."))
	case "error":
		sb.WriteString(styleRed.Render("  Update failed"))
		sb.WriteString("\n\n")
		sb.WriteString(styleHeaderDim.Render(fmt.Sprintf("  %s", errMsg)))
		sb.WriteString("\n\n")
		sb.WriteString(styleHeaderDim.Render("  You can update manually:"))
		sb.WriteString("\n")
		binaryName := fmt.Sprintf("bot-%s-%s", runtime.GOOS, runtime.GOARCH)
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		sb.WriteString(styleKeybindKey.Render(fmt.Sprintf("  curl -Lo bot %s/%s", releaseBaseURL, binaryName)))
	default:
		sb.WriteString(styleHeaderValue.Render("  Preparing update..."))
	}

	return sb.String()
}
