package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
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

func actionInputArgs(in io.Reader, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > 1 || (len(args) == 0 && standardInputIsTerminal(in)) {
			return newUsageError(fmt.Errorf("usage: %s", usage))
		}
		return nil
	}
}

func actionOrTargetsArgs(in io.Reader, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) == 0 && standardInputIsTerminal(in) {
			return newUsageError(fmt.Errorf("usage: %s", usage))
		}
		return nil
	}
}

// consumeActionRecords 在一条 operation snapshot 上顺序执行记录动作；它不缓存
// 整个输入，因此大管道只保留当前 Record。
func (a app) consumeActionRecords(cmd *cobra.Command, operation, onError string, allowedTypes map[string]struct{}, invoke func(context.Context, int64) error) error {
	failFast, err := recordFailureStrategy(onError)
	if err != nil {
		return err
	}
	return consumeNDJSONRecords(cmd.Context(), a.in, a.errOut, operation, failFast, func(ctx context.Context, record application.Record) error {
		if _, ok := allowedTypes[record.Type()]; !ok {
			return newRecordActionError("unsupported_type", fmt.Errorf("record type %q is not supported by %s", record.Type(), operation))
		}
		id, err := requiredRecordID(record)
		if err != nil {
			return newRecordActionError("invalid_id", err)
		}
		if err := invoke(ctx, id); err != nil {
			return newRecordActionError("action_failed", err)
		}
		return nil
	})
}

var visualRecordTypes = map[string]struct{}{
	"illust": {},
	"manga":  {},
	"ugoira": {},
}

var userRecordTypes = map[string]struct{}{
	"user": {},
}
