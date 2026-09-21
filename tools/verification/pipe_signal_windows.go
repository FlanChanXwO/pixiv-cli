//go:build windows

package main

func processTerminatedBySIGPIPE(error) bool {
	return false
}
