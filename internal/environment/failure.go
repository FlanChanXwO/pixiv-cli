package environment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/report"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
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
func ErrorText(err error, language i18n.Locale) (text string) {
	defer func() {
		if strings.IndexFunc(text, unicode.IsControl) >= 0 {
			text = display.SafeLine(text)
		}
	}()
	for current := err; current != nil; current = errors.Unwrap(current) {
		// 反搜错误已在服务边界映射；底层 SDK 原因只供 errors.Is/As 诊断。
		if _, ok := current.(*reversesearch.Error); ok {
			break
		}
		if joined, ok := current.(interface{ Unwrap() []error }); ok {
			var replacements []string
			for _, cause := range joined.Unwrap() {
				replacements = append(replacements, language.ErrorText(cause), ErrorText(cause, language))
			}
			message := strings.NewReplacer(replacements...).Replace(language.ErrorText(err))
			return strings.ReplaceAll(message, "\n", "; ")
		}
	}
	if message, ok := flagErrorText(err, language); ok {
		return message
	}
	if message, ok := commandErrorText(err, language); ok {
		return message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return language.Text("error.timeout")
	}
	if errors.Is(err, context.Canceled) {
		return language.Text("error.canceled")
	}
	if fanbox, ok := err.(*report.Error); ok {
		if fanbox.IsAuthentication() {
			return language.Text("error.fanbox_login")
		}
		return language.ErrorText(err)
	}
	var reverse *reversesearch.Error
	if errors.As(err, &reverse) {
		return language.ErrorText(err)
	}
	var current *sdk.Error
	if !errors.As(err, &current) {
		return language.ErrorText(err)
	}
	reason, detail, httpStatus := current.Reason, current.Message, current.HTTPStatus
	switch reason {
	case sdk.Unauthorized, sdk.CredentialsExpired:
		return language.Text("error.not_logged_in_run_pixiv_auth_login")
	case sdk.InvalidArgument:
		if httpStatus == 0 {
			if detail == "" {
				return language.Text("error.invalid_argument")
			}
			return fmt.Sprintf(language.Text("error.argument_detail"), detail)
		}
	}
	if httpStatus != 0 {
		return fmt.Sprintf(language.Text("format.upstream_returned_http_check_service_availability_and_account"), httpStatus, http.StatusText(httpStatus))
	}
	return language.ErrorText(err)
}

// commandErrorText 按 Cobra 的固定格式翻译未知命令；Cobra 未导出对应错误类型。
func commandErrorText(err error, locale i18n.Locale) (string, bool) {
	for current := err; current != nil; current = errors.Unwrap(current) {
		line, detail, found := strings.Cut(current.Error(), "\n")
		var command, path string
		if _, scanErr := fmt.Sscanf(line, "unknown command %q for %q", &command, &path); scanErr != nil || line != fmt.Sprintf("unknown command %q for %q", command, path) {
			continue
		}
		message := fmt.Sprintf(locale.Text("error.unknown_command"), command, path)
		if found {
			message += "\n" + strings.Replace(detail, "\nDid you mean this?\n", "\n"+locale.Text("error.command_suggestions")+"\n", 1)
		}
		return strings.Replace(locale.ErrorText(err), current.Error(), message, 1), true
	}
	return "", false
}
