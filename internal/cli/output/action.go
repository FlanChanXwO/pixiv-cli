package output

import (
	"context"
	"fmt"
	"io"
	"os"

	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// standardInputIsTerminal 只在真实字符设备上要求人工提供位置参数；嵌入式调用
// 使用的 reader 视为管道，以便测试和其他程序直接组合 CLI。
var standardInputIsTerminal = func(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

// ActionInputArgs 校验最多一个显式 ID；没有位置参数时仅允许真实管道输入。
// usageError 由根控制器注入，以保留 CLI 的 usage exit code 语义。
func ActionInputArgs(in io.Reader, usage string, usageError func(error) error) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > 1 || (len(args) == 0 && standardInputIsTerminal(in)) {
			return usageError(fmt.Errorf("usage: %s", usage))
		}
		return nil
	}
}

// ActionOrTargetsArgs 允许显式资源，也允许从 stdin 读取 NDJSON 记录。
func ActionOrTargetsArgs(in io.Reader, usage string, usageError func(error) error) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) == 0 && standardInputIsTerminal(in) {
			return usageError(fmt.Errorf("usage: %s", usage))
		}
		return nil
	}
}

// ConsumeActionRecords 在一条 operation snapshot 上顺序执行记录动作；它不缓存
// 整个输入，因此大管道只保留当前 Record。
func ConsumeActionRecords(ctx context.Context, in io.Reader, errOut io.Writer, operation, onError string, allowedTypes map[string]struct{}, invoke func(context.Context, int64) error, usageError func(error) error) error {
	failFast, err := RecordFailureStrategy(onError)
	if err != nil {
		return usageError(err)
	}
	return ConsumeNDJSONRecords(ctx, in, errOut, operation, failFast, func(ctx context.Context, record recordpkg.Record) error {
		if _, ok := allowedTypes[record.Type()]; !ok {
			return NewRecordActionError("unsupported_type", fmt.Errorf("record type %q is not supported by %s", record.Type(), operation))
		}
		id, err := RequiredRecordID(record)
		if err != nil {
			return NewRecordActionError("invalid_id", err)
		}
		if err := invoke(ctx, id); err != nil {
			return NewRecordActionError("action_failed", err)
		}
		return nil
	})
}
