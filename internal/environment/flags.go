package environment

import (
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// BindNDJSONFlag 注册列表命令自己的记录流开关。
func BindNDJSONFlag(cmd *cobra.Command, value *bool) {
	cmd.Flags().BoolVar(value, "ndjson", false, "flag.print_one_pixiv_entity_record_as_json_per")
}

// BindListFlags 给列表命令注册逻辑分页 flag；未给 --limit 时只读一批。
func BindListFlags(cmd *cobra.Command, limit, page *int) {
	flags := cmd.Flags()
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVarP(limit, "limit", "l", 0, "flag.maximum_results_omitted_returns_one_upstream_batch_0")
	flags.IntVarP(page, "page", "p", 0, "flag.1_based_logical_page_requires_limit_0")
}

// BindFanboxListFlags 给 FANBOX 列表命令注册输出格式和逻辑分页 flag。
// FANBOX 的 --limit/--page 没有短选项，帮助文本与既有输出保持一致。
func BindFanboxListFlags(cmd *cobra.Command, jsonOut, ndjson *bool, limit, page *int) {
	flags := cmd.Flags()
	flags.BoolVar(jsonOut, "json", false, "flag.print_json")
	flags.BoolVar(ndjson, "ndjson", false, "flag.print_one_item_as_json_per_line")
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVar(limit, "limit", 0, "flag.maximum_results_omitted_returns_one_upstream_batch_0")
	flags.IntVar(page, "page", 0, "flag.1_based_logical_page_requires_limit_0")
}

// BindFanboxSingleFlags 只给单实体命令注册 --json/--ndjson，避免接受会被静默忽略的 --limit/--page。
func BindFanboxSingleFlags(cmd *cobra.Command, jsonOut, ndjson *bool) {
	flags := cmd.Flags()
	flags.BoolVar(jsonOut, "json", false, "flag.print_json")
	flags.BoolVar(ndjson, "ndjson", false, "flag.print_one_item_as_json_per_line")
}

// ParsePlan 校验 --limit/--page 并生成逻辑分页计划；未显式给 --limit 时只读一批。
func ParsePlan(cmd *cobra.Command, limit, page int) (pagination.PagePlan, error) {
	return pagination.ParsePlan(limit, page, cmd.Flags().Changed("limit"), cmd.Flags().Changed("page"))
}

// ListJSONOut 解析文档输出模式；同时给 --ndjson 和 --json 时按 usage 错误拒绝。
func (e Env) ListJSONOut(cmd *cobra.Command, jsonOut, ndjson bool) (bool, error) {
	if ndjson && cmd.Flags().Changed("json") {
		return false, e.WrapUsage(i18n.Errorf("error.ndjson_cannot_be_used_with_json"))
	}
	return !ndjson && jsonOut, nil
}

// Request 从共用代理 flag 构造一次 Pixiv 执行请求。
func (e Env) Request(cmd *cobra.Command, opts ProxyOptions) (PixivRequest, error) {
	proxy, err := ProxyOverride(cmd, opts)
	if err != nil {
		return PixivRequest{}, err
	}
	return PixivRequest{HTTPSProxyOverride: proxy}, nil
}

// NormalizeFlagError 把 pflag 的未知选项错误翻译成 usage 错误；其他错误原样返回。
func NormalizeFlagError(err error) error {
	var notExist *pflag.NotExistError
	if !errors.As(err, &notExist) {
		return err
	}
	if shortnames := notExist.GetSpecifiedShortnames(); shortnames != "" {
		return NewUsageError(i18n.Errorf("error.unknown_option", []rune(shortnames)[0]))
	}
	if name := notExist.GetSpecifiedName(); name != "" {
		return NewUsageError(i18n.Errorf("error.unknown_option_545", name))
	}
	return err
}

// flagErrorText 翻译 pflag 的应用提示，底层解析原因及原始错误值保持完整。
func flagErrorText(err error, locale i18n.Locale) (string, bool) {
	switch failure := err.(type) {
	case *pflag.ValueRequiredError:
		if short := failure.GetSpecifiedShortnames(); short != "" {
			return fmt.Sprintf(locale.Text("error.flag_value_required_short"), []rune(failure.GetSpecifiedName())[0], short), true
		}
		return fmt.Sprintf(locale.Text("error.flag_value_required"), failure.GetSpecifiedName()), true
	case *pflag.InvalidValueError:
		flag := failure.GetFlag()
		name := "--" + flag.Name
		if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
			name = "-" + flag.Shorthand + ", " + name
		}
		return fmt.Sprintf(locale.Text("error.flag_invalid_value"), failure.GetValue(), name, locale.ErrorText(errors.Unwrap(failure))), true
	case *pflag.InvalidSyntaxError:
		return fmt.Sprintf(locale.Text("error.flag_invalid_syntax"), failure.GetSpecifiedFlag()), true
	}
	return "", false
}
