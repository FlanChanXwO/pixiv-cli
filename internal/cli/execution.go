package cli

// 本文件负责一次 CLI 执行的入口与生命周期：参数入口（Run/RunContext*）、
// 退出码、命令判定（usage/flag 类别）、资源关闭、启动钩子与诊断启停。
//
// 它不构造业务依赖——那属于 composition.go；也不定义命令树——那属于 root.go。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"syscall"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth"
	clidiagnostics "github.com/FlanChanXwO/pixiv-cli/internal/cli/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/invocation"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	coreDiagnostics "github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

type diagnosticState struct {
	ctx       context.Context
	operation string
	presenter *clidiagnostics.Presenter
}

// closeState tracks resources opened by the current invocation. It is only a
// reverse-order close list; it does not cache services or expose a graph.
type closeState struct {
	mu      sync.Mutex
	closers []func() error
	err     error
	once    sync.Once
}

func (s *closeState) add(closer func() error) {
	if s == nil || closer == nil {
		return
	}
	s.mu.Lock()
	s.closers = append(s.closers, closer)
	s.mu.Unlock()
}

func (s *closeState) close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for index := len(s.closers) - 1; index >= 0; index-- {
			s.err = errors.Join(s.err, s.closers[index]())
		}
	})
	return s.err
}

// brokenPipeSignalState 临时安装平台的 broken-pipe 信号策略，使 stdout 写失败能以
// EPIPE 返回 Go 调用链。NDJSON 与 MCP 使用独立状态，且只有前者可把 EPIPE 归为成功。
type brokenPipeSignalState struct {
	enable func() func()
	stop   func()
}

func Run(args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	return runContext(context.Background(), args, in, out, errOut, nil, nil)
}

// RunContext 让嵌入式调用方把取消信号传到每一条网络数据命令。
func RunContext(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	return runContext(ctx, args, in, out, errOut, nil, nil)
}

// RunContextWithPipelineSignal 仅由二进制入口传入 SIGPIPE 控制器。控制器会在
// filter 或已解析的 --ndjson 查询命令运行期间启用，并在命令退出时恢复。
func RunContextWithPipelineSignal(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer, enablePipelineSignal func() func()) int {
	return RunContextWithBrokenPipeSignals(ctx, args, in, out, errOut, enablePipelineSignal, nil)
}

// RunContextWithBrokenPipeSignals 仅供二进制入口传入平台的 SIGPIPE 控制器。普通
// NDJSON 输出和 MCP stdio 必须分别传入控制器：前者的 EPIPE 是下游正常停止，后者
// 则是 JSON-RPC transport 错误。
func RunContextWithBrokenPipeSignals(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer, enablePipelineSignal, enableMCPBrokenPipeSignal func() func()) int {
	var pipelineSignal, mcpBrokenPipeSignal *brokenPipeSignalState
	if enablePipelineSignal != nil {
		pipelineSignal = &brokenPipeSignalState{enable: enablePipelineSignal}
	}
	if enableMCPBrokenPipeSignal != nil {
		mcpBrokenPipeSignal = &brokenPipeSignalState{enable: enableMCPBrokenPipeSignal}
	}
	return runContext(ctx, args, in, out, errOut, pipelineSignal, mcpBrokenPipeSignal)
}

// RunContextWithDefaultBrokenPipeSignals 为二进制入口装配当前平台的默认 SIGPIPE
// 控制器：普通 NDJSON 输出与 MCP stdio 各自独立。嵌入式调用方若需要自定义信号
// 策略，应直接调用 RunContext 或 RunContextWithBrokenPipeSignals。
func RunContextWithDefaultBrokenPipeSignals(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	return RunContextWithBrokenPipeSignals(ctx, args, in, out, errOut, enablePipelineBrokenPipeSignal, enableMCPBrokenPipeSignal)
}

func runContext(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer, pipelineSignal, mcpBrokenPipeSignal *brokenPipeSignalState) int {
	if len(args) == 0 {
		args = []string{"pixiv"}
	}
	streams := invocation.NewStreams(in, out, errOut)
	a := app{
		in:                  streams.In,
		out:                 streams.Out,
		errOut:              streams.Err,
		pipelineSignal:      pipelineSignal,
		mcpBrokenPipeSignal: mcpBrokenPipeSignal,
		closeState:          &closeState{},
		diagnostics:         &diagnosticState{},
	}
	if pipelineSignal != nil {
		defer func() {
			if pipelineSignal.stop != nil {
				pipelineSignal.stop()
			}
		}()
	}
	if mcpBrokenPipeSignal != nil {
		defer func() {
			if mcpBrokenPipeSignal.stop != nil {
				mcpBrokenPipeSignal.stop()
			}
		}()
	}
	cmd := a.newRootCommand()
	defer pipeline.Clear(cmd)
	defer authcommands.ClearInputState(cmd)
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args[1:])
	cmd.SetContext(ctx)
	target := cmd
	if found, _, findErr := cmd.Find(args[1:]); findErr == nil && found != nil {
		target = found
	}
	err := cmd.Execute()
	if closeErr := a.closeResources(); closeErr != nil {
		if err == nil {
			err = closeErr
		} else {
			err = errors.Join(err, closeErr)
		}
	}
	err = a.finishDiagnostics(err)
	return a.exitWithNDJSONScope(err, commandWritesNDJSON(target) || commandAutoWritesNDJSON(target, out))
}

func (a app) exitWithNDJSONScope(err error, ndjsonOutput bool) int {
	if err == nil {
		return 0
	}
	if ndjsonOutput && errors.Is(err, syscall.EPIPE) {
		return 0
	}
	var startupErr *startupError
	if errors.As(err, &startupErr) {
		fmt.Fprintln(a.errOut, startupErr.err)
		return 1
	}
	var usageErr *usageError
	if errors.As(err, &usageErr) {
		fmt.Fprintln(a.errOut, "error:", usageErr.err)
		return 2
	}
	var pipelineErr *pipeline.PipelineDiagnosticError
	if errors.As(err, &pipelineErr) {
		return 1
	}
	fmt.Fprintln(a.errOut, "error:", err)
	return 1
}

// startupError 保持解析成功后的启动副作用失败契约：它是命令启动失败，
// 不是用户参数错误，因此使用普通 process-level exit code 1 且不伪装成 usage。
type startupError struct{ err error }

func (e *startupError) Error() string { return e.err.Error() }

func (e *startupError) Unwrap() error { return e.err }

func normalizeFlagError(err error) error {
	var notExist *pflag.NotExistError
	if !errors.As(err, &notExist) {
		return err
	}

	if shortnames := notExist.GetSpecifiedShortnames(); shortnames != "" {
		return newUsageError(fmt.Errorf("unknown option '-%c'", []rune(shortnames)[0]))
	}
	if name := notExist.GetSpecifiedName(); name != "" {
		return newUsageError(fmt.Errorf("unknown option '--%s'", name))
	}
	return err
}

func commandWritesNDJSON(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flags().Lookup("ndjson")
	return flag != nil && flag.Changed && flag.Value.String() == "true"
}

// commandAutoWritesNDJSON 与各视觉列表实际的 stdout 判定保持一致，让下游主动
// 关闭管道时可按 Unix 习惯结束而不把 EPIPE 误报为命令失败。
func commandAutoWritesNDJSON(cmd *cobra.Command, out io.Writer) bool {
	if cmd == nil || cmd.Flags().Changed("json") {
		return false
	}
	if cmd.Annotations != nil {
		if value, ok := cmd.Annotations["pixiv-cli.output-ndjson"]; ok {
			return value == "true"
		}
	}
	file, ok := out.(interface{ Fd() uintptr })
	if !ok || term.IsTerminal(int(file.Fd())) {
		return false
	}
	return slices.Contains([]string{
		"pixiv search", "pixiv ranking", "pixiv recommended",
		"pixiv timeline following", "pixiv timeline latest",
		"pixiv mypixiv works", "pixiv user artworks", "pixiv user bookmarks",
	}, cmd.CommandPath())
}

// usageError 标记由 CLI 参数、flag 或显式输入契约验证导致的错误；上游 SDK、
// 网络和本地 I/O 错误不得包裹为此类型，以保持 shell 可区分的退出码语义。
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }

func (e *usageError) Unwrap() error { return e.err }

func newUsageError(err error) error {
	if err == nil {
		return nil
	}
	var existing *usageError
	if errors.As(err, &existing) {
		return err
	}
	return &usageError{err: err}
}

func (a app) closeResources() error {
	return a.closeState.close()
}

func registerReverseSearchCloser(state *closeState, searcher reversesearch.Searcher) {
	if closer, ok := searcher.(reversesearch.Closer); ok {
		state.add(closer.Close)
	}
}

func (a app) startDiagnostics(cmd *cobra.Command, requirement requirements.Execution) error {
	if a.diagnostics == nil || !requirement.EnsureConfig || isQuietConfigCommand(cmd) {
		return nil
	}
	runtime, err := a.runtimeConfig()
	if err != nil {
		return err
	}
	if runtime.LogLevel != "debug" {
		return nil
	}
	module := coreDiagnostics.ModulePixivCLI
	if strings.HasPrefix(cmd.CommandPath(), "pixiv fanbox") {
		module = coreDiagnostics.ModuleFanboxCLI
	}
	presenter := clidiagnostics.NewPresenterWithFormat(a.errOut, runtime.LogFormat, nil)
	scoped := coreDiagnostics.WithScope(cmd.Context(), presenter, module, 0)
	a.diagnostics.ctx = scoped
	a.diagnostics.operation = cmd.CommandPath()
	a.diagnostics.presenter = presenter
	coreDiagnostics.Emit(scoped, coreDiagnostics.Event{
		Kind:      coreDiagnostics.EventStarted,
		Operation: cmd.CommandPath(),
	})
	cmd.SetContext(scoped)
	return nil
}

func (a app) finishDiagnostics(err error) error {
	if a.diagnostics == nil || a.diagnostics.presenter == nil {
		return err
	}
	event := coreDiagnostics.Event{Operation: a.diagnostics.operation}
	if err == nil {
		event.Kind = coreDiagnostics.EventCompleted
	} else {
		event.Kind = coreDiagnostics.EventFailed
		event.Reason = coreDiagnostics.ReasonCommandFailed
	}
	coreDiagnostics.Emit(a.diagnostics.ctx, event)
	if diagnosticErr := a.diagnostics.presenter.Err(); diagnosticErr != nil {
		if err == nil {
			return fmt.Errorf("write diagnostics: %w", diagnosticErr)
		}
		return errors.Join(err, fmt.Errorf("write diagnostics: %w", diagnosticErr))
	}
	return err
}

func isQuietConfigCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return cmd.CommandPath() == "pixiv config" || strings.HasPrefix(cmd.CommandPath(), "pixiv config ")
}

func (a app) enablePipelineSignal(cmd *cobra.Command) {
	if a.pipelineSignal == nil || a.pipelineSignal.enable == nil || a.pipelineSignal.stop != nil || !commandWritesNDJSON(cmd) {
		return
	}
	a.pipelineSignal.stop = a.pipelineSignal.enable()
}

func (a app) enableMCPBrokenPipeSignal(requirement requirements.Execution) {
	if a.mcpBrokenPipeSignal == nil || a.mcpBrokenPipeSignal.enable == nil || a.mcpBrokenPipeSignal.stop != nil || !requirement.MCP {
		return
	}
	a.mcpBrokenPipeSignal.stop = a.mcpBrokenPipeSignal.enable()
}
