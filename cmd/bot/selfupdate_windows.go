//go:build windows

package main

import "errors"

var errWindowsRestart = errors.New("windows_restart_required")

func reExecSelf() error {
	return errWindowsRestart
}
