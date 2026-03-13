//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func reExecSelf() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not find binary: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("could not resolve binary: %w", err)
	}
	return syscall.Exec(execPath, os.Args, os.Environ())
}
