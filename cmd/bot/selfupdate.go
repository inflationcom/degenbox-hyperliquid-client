package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

const releaseBaseURL = "https://github.com/inflationcom/degenbox-hyperliquid-client/releases/latest/download"

func buildDownloadURL() string {
	name := fmt.Sprintf("bot-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return releaseBaseURL + "/" + name
}

func performUpdate() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not find binary path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("could not resolve binary path: %w", err)
	}

	url := buildDownloadURL()

	// Create temp file in the same directory (same filesystem for atomic rename)
	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, "bot-update-*")
	if err != nil {
		return fmt.Errorf("could not create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // cleanup on error; on success the rename consumed it

	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		tmp.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("download failed: HTTP %d from %s", resp.StatusCode, url)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("download incomplete: %w", err)
	}
	tmp.Close()

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			return fmt.Errorf("chmod failed: %w", err)
		}
	}

	// Replace the binary
	if runtime.GOOS == "windows" {
		// Windows can't overwrite a running binary — rename current to .old first
		oldPath := execPath + ".old"
		os.Remove(oldPath) // remove any previous .old
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("could not rename current binary: %w", err)
		}
		if err := os.Rename(tmpPath, execPath); err != nil {
			// Rollback
			os.Rename(oldPath, execPath)
			return fmt.Errorf("could not install new binary: %w", err)
		}
	} else {
		if err := os.Rename(tmpPath, execPath); err != nil {
			return fmt.Errorf("could not replace binary: %w", err)
		}
	}

	return nil
}
