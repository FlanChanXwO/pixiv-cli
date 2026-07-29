//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package main

func enablePipelineBrokenPipeSignal() func() { return func() {} }

func enableMCPBrokenPipeSignal() func() { return func() {} }
