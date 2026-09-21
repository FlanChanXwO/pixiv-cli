//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"syscall"
)

func processTerminatedBySIGPIPE(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGPIPE
}
