package environment

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/spf13/cobra"
)

// UsageError 标记由 CLI 参数、flag 或显式输入契约验证导致的错误；上游 SDK、
// 网络和本地 I/O 错误不得包裹为此类型，以保持 shell 可区分的退出码语义。
type UsageError struct{ Err error }

// Error 返回参数校验的原始错误文本。
func (e *UsageError) Error() string { return e.Err.Error() }

// Unwrap 暴露参数校验的原始错误。
func (e *UsageError) Unwrap() error { return e.Err }

// NewUsageError 给参数错误打上 usage 标记；已经带标记的错误原样返回。
func NewUsageError(err error) error {
	if err == nil {
		return nil
	}
	var existing *UsageError
	if errors.As(err, &existing) {
		return err
	}
	return &UsageError{Err: err}
}

// StartupError 标记命令启动阶段的副作用失败。它不使用 usage 退出码，也不打印
// "error:" 前缀，调用方据此区分启动失败与参数错误。
type StartupError struct{ Err error }

// Error 返回启动副作用的原始错误文本。
func (e *StartupError) Error() string { return e.Err.Error() }

// Unwrap 暴露启动失败的原始错误。
func (e *StartupError) Unwrap() error { return e.Err }

// WritesNDJSON 只认命令自己 --ndjson flag 的显式取值；自动 NDJSON 由 command owner
// 通过 SetOutputNDJSON 记录。
func WritesNDJSON(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("ndjson")
	return flag != nil && flag.Changed && flag.Value.String() == "true"
}

// ErrorText 在 CLI 边界解释常见 SDK 错误，保留原始错误值供结构化调用方使用。
func ErrorText(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "request timed out; check your connection and try again"
	}
	if errors.Is(err, context.Canceled) {
		return "operation canceled; run the command again when ready"
	}
	var failure *sdk.Error
	if !errors.As(err, &failure) {
		return err.Error()
	}
	switch failure.Reason {
	case sdk.Unauthorized, sdk.CredentialsExpired:
		login := "pixiv auth login"
		if failure.Product == "fanbox" {
			login = "pixiv fanbox auth login"
		}
		return fmt.Sprintf("not logged in; run %q", login)
	case sdk.InvalidArgument, sdk.InvalidCursor:
		return "invalid argument: " + failure.Detail + "; check the command's --help"
	}
	if failure.HTTPStatus != 0 {
		return fmt.Sprintf("upstream returned HTTP %d (%s); check service availability and account access before trying again", failure.HTTPStatus, http.StatusText(failure.HTTPStatus))
	}
	return err.Error()
}
