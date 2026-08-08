package update

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// NewCommandRunner 建立把子进程输出直连到 CLI 输出流的无 shell 命令执行器。
func NewCommandRunner(out, errOut io.Writer) CommandRunner {
	return processCommandRunner{out: out, errOut: errOut}
}

type processCommandRunner struct {
	out    io.Writer
	errOut io.Writer
}

func (r processCommandRunner) Run(ctx context.Context, command Command) error {
	if command.Name == "" {
		return fmt.Errorf("command name is empty")
	}
	process := exec.CommandContext(ctx, command.Name, command.Args...)
	process.Stdout = r.out
	process.Stderr = r.errOut
	if err := process.Run(); err != nil {
		return fmt.Errorf("run command %q: %w", command.Name, err)
	}
	return nil
}
