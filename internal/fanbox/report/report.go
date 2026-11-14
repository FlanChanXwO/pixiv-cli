// Package report 保留 FANBOX 命令和工具的错误诊断格式，原始 SDK 错误可通过 Unwrap 检查。
package report

import (
	"errors"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Error 为 FANBOX 的呈现边界保留产品身份和既有会话、挑战错误文本。
type Error struct {
	Err       error
	operation string
}

// Error 输出受控诊断，不读取 SDK 错误的原始外部载荷。
func (e *Error) Error() string {
	var failure *sdk.Error
	if !errors.As(e.Err, &failure) {
		return e.Err.Error()
	}
	operation := e.operation
	if operation == "" {
		operation = failure.Operation
	}
	switch operation {
	case "Post.Get":
		operation = "Post"
	case "Creator.Get":
		operation = "Creator"
	case "Creator.Tags":
		operation = "CreatorTags"
	case "Creator.Posts":
		operation = "CreatorPosts"
	case "ParseURL":
		operation = "ResolveURL"
	}
	reason := ReasonOf(failure)
	message := failure.Message
	switch failure.Reason {
	case sdk.CredentialsExpired:
		message = "fanbox: not authenticated"
	case sdk.ChallengeRequired:
		message = "fanbox: challenge required"
	}
	if failure.Operation == "cloudflare.Solve" {
		switch reason {
		case sdk.ChallengeRequired:
			message = "fanbox: FlareSolverr could not solve challenge"
		case sdk.MalformedUpstreamResponse:
			message = "fanbox: malformed FlareSolverr response"
		case sdk.UpstreamUnavailable:
			message = "fanbox: FlareSolverr service unavailable"
		}
	}
	text := "fanbox:" + operation + ": " + string(reason)
	if message != "" {
		text += ": " + message
	}
	return strings.Replace(e.Err.Error(), failure.Error(), text, 1)
}

// Unwrap 保留取消、HTTP 状态与 SDK 分类。
func (e *Error) Unwrap() error { return e.Err }

// Wrap 在应用边界标记 FANBOX 错误；operation 保留产品调用名称，空值沿用 SDK 名称。
// nil 与已标记的错误保持原样，底层分类与联合错误的全部原因保留。
func Wrap(err error, operation string) error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		wrapped := make([]error, len(causes))
		for i, cause := range causes {
			wrapped[i] = Wrap(cause, operation)
		}
		return errors.Join(wrapped...)
	}
	var reported *Error
	var failure *sdk.Error
	if !errors.As(err, &failure) {
		return err
	}
	if errors.As(err, &reported) {
		return err
	}
	return &Error{Err: err, operation: operation}
}

// IsAuthentication 判断已标记错误是否需要重新导入 FANBOX 会话。
func (e *Error) IsAuthentication() bool {
	reason := sdk.ReasonOf(e.Err)
	return reason == sdk.Unauthorized || reason == sdk.CredentialsExpired
}

// ReasonOf 保留工具的既有求解失败分类；原 SDK 错误及 HTTP 状态仍可通过 Unwrap 检查。
func ReasonOf(err error) sdk.Reason {
	var failure *sdk.Error
	if !errors.As(err, &failure) {
		return ""
	}
	if failure.Operation == "cloudflare.Solve" && failure.HTTPStatus != 0 {
		return sdk.ChallengeRequired
	}
	return failure.Reason
}
