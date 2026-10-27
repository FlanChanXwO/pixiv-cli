package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"syscall"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// logState 保存本次执行的 logger；命令结束后写一条终止 ERROR。
type logState struct {
	ctx    context.Context
	logger *slog.Logger
}

// closeState 仅登记一次 CLI 执行的逆序关闭函数，不缓存服务或暴露依赖图。
//
// 契约：
//   - 所有权：谁打开资源，谁调用 add 登记关闭函数；资源在登记前不得被其他人假定已可用。
//   - 关闭顺序：close 按登记的**逆序**关闭（后打开的先关闭），符合依赖关系。
//   - 幂等：close 只真正执行一次（sync.Once），重复调用返回同一结果。
//   - 错误聚合：关闭错误全部并入同一个 error，不因第一个失败而漏关其余资源。
//   - 生命周期：调用方须先停止资源生产者、完成所有 add，再调用 close；不得在关闭开始后再登记。
//     runContext 在命令返回后关闭；MCP Run 等待在途 handler 结束后才返回。
//   - 锁只保护登记表，不协调生产者退出；closer 在持锁期间执行，不能对同一 scope 调用 add 或 close。
type closeState struct {
	mu      sync.Mutex
	closers []func() error
	err     error
	once    sync.Once
}

// add 登记一个关闭函数；调用点只在确实打开资源时登记。
func (s *closeState) add(closer func() error) {
	s.mu.Lock()
	s.closers = append(s.closers, closer)
	s.mu.Unlock()
}

// close 在生产者退出、登记完成后逆序关闭资源并返回聚合错误；它是幂等的。
func (s *closeState) close() error {
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

func (s *brokenPipeSignalState) restore() {
	if s.stop != nil {
		s.stop()
	}
}

// Run 执行一条 CLI 命令并返回进程退出码，阶段顺序固定：
//
// 1. 只用输入输出流和资源登记表构造 app，不打开资源；
// 2. 装配命令树并绑定流与参数；
// 3. 执行命令，资源在命令真正请求时才创建，help、version 不会提前打开数据库或网络 client；
// 4. 无论成功与否都按登记逆序关闭资源，关闭错误并入退出原因；
// 5. 结束诊断并映射退出码。
//
// 普通 NDJSON 输出与 MCP stdio 各自持有平台的 SIGPIPE 控制器：前者的 EPIPE 表示下游
// 正常停止，后者属于 JSON-RPC transport 错误。
func Run(ctx context.Context, args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	if len(args) == 0 {
		args = []string{"pixiv"}
	}
	// 阶段 1：只用流与状态容器构造 app；closeState 在此登记，资源稍后按需打开。
	a := app{
		in:                  in,
		out:                 out,
		errOut:              errOut,
		pipelineSignal:      &brokenPipeSignalState{enable: enablePipelineBrokenPipeSignal},
		mcpBrokenPipeSignal: &brokenPipeSignalState{enable: enableMCPBrokenPipeSignal},
		closeState:          &closeState{},
		logs:                &logState{ctx: ctx, logger: logging.Discard()},
	}
	defer a.pipelineSignal.restore()
	defer a.mcpBrokenPipeSignal.restore()
	// 阶段 2：装配命令树并绑定流/参数。
	cmd := a.newRootCommand()
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args[1:])
	cmd.SetContext(ctx)
	target := cmd
	if found, _, findErr := cmd.Find(args[1:]); findErr == nil && found != nil {
		target = found
	}
	// 阶段 3：执行命令，按需创建资源；启动钩子的文件/系统副作用由命令需求决定。
	err := cmd.Execute()
	// 阶段 4：关闭资源。即使执行失败也要关闭，并把关闭错误并入退出原因，
	// 避免"命令成功但资源未释放"被报成成功。
	if closeErr := a.closeResources(); closeErr != nil {
		if err == nil {
			err = closeErr
		} else {
			err = errors.Join(err, closeErr)
		}
	}
	// 阶段 5：结束诊断并映射退出码（NDJSON 命令走成功语义）。
	err = a.finishLogging(err)
	return a.exitWithNDJSONScope(err, commandWritesNDJSON(target) || command.OutputNDJSON(target))
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
	var pipelineErr *input.PipelineDiagnosticError
	if errors.As(err, &pipelineErr) {
		return 1
	}
	fmt.Fprintln(a.errOut, "error:", err)
	return 1
}

// startupError 标记命令启动阶段的副作用失败。它不使用 usage 退出码，也不打印
// "error:" 前缀，调用方据此区分启动失败与参数错误。
type startupError struct{ err error }

// Error 返回启动副作用的原始错误文本。
func (e *startupError) Error() string { return e.err.Error() }

// Unwrap 暴露启动失败的原始错误。
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

// commandWritesNDJSON 只认命令自己 --ndjson flag 的显式取值；自动 NDJSON 由 command owner
// 通过 command.SetOutputNDJSON 记录。
func commandWritesNDJSON(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("ndjson")
	return flag != nil && flag.Changed && flag.Value.String() == "true"
}

// usageError 标记由 CLI 参数、flag 或显式输入契约验证导致的错误；上游 SDK、
// 网络和本地 I/O 错误不得包裹为此类型，以保持 shell 可区分的退出码语义。
type usageError struct{ err error }

// Error 返回参数校验的原始错误文本。
func (e *usageError) Error() string { return e.err.Error() }

// Unwrap 暴露参数校验的原始错误。
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

func (a app) startLogging(cmd *cobra.Command, requirement command.Execution) error {
	if !requirement.EnsureConfig || isQuietConfigCommand(cmd) {
		return nil
	}
	runtime, err := a.runtimeConfig()
	if err != nil {
		return err
	}
	logger, err := logging.New(a.errOut, string(runtime.LogLevel), string(runtime.LogFormat))
	if err != nil {
		return err
	}
	ctx := logging.WithCall(cmd.Context(), logger, cmd.CommandPath(), "")
	a.logs.ctx, a.logs.logger = ctx, logger
	cmd.SetContext(ctx)
	return nil
}

func (a app) finishLogging(err error) error {
	if err == nil {
		return err
	}
	a.logs.logger.ErrorContext(a.logs.ctx, "Command failed", slog.Any(logging.AttrError, err))
	return err
}

func isQuietConfigCommand(cmd *cobra.Command) bool {
	return cmd.CommandPath() == "pixiv config" || strings.HasPrefix(cmd.CommandPath(), "pixiv config ")
}

// enablePipelineSignal 安装 NDJSON 专用的 SIGPIPE 策略。command owner 只在确定输出记录流时
// 调用，其他命令保留平台的默认 SIGPIPE 行为，否则普通输出被关闭管道截断会被误判为成功。
func (a app) enablePipelineSignal() {
	if a.pipelineSignal.stop != nil {
		return
	}
	a.pipelineSignal.stop = a.pipelineSignal.enable()
}

func (a app) enableMCPBrokenPipeSignal(requirement command.Execution) {
	if a.mcpBrokenPipeSignal.stop != nil || !requirement.MCP {
		return
	}
	a.mcpBrokenPipeSignal.stop = a.mcpBrokenPipeSignal.enable()
}
