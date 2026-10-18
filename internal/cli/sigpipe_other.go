//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package cli

func enablePipelineBrokenPipeSignal() func() { return func() {} }

func enableMCPBrokenPipeSignal() func() { return func() {} }
