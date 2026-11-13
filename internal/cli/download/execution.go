package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/saving"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk/download"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type controller struct {
	env     environment.Env
	execute session.SDKExecutor
	runtime func() (Runtime, error)
}

type commandOptions struct {
	proxyOptions
	jsonOut bool
}

type proxyOptions struct {
	proxy   string
	noProxy bool
}

type downloadOptions struct {
	commandOptions
	input            *environment.State
	downloadPath     string
	outputPath       string
	filenameTemplate string
	pages            string
	quality          string
	ugoiraMode       string
	onError          string
}

var visualRecordTypes = map[string]struct{}{
	string(target.NamespaceArtwork): {},
}

func (a controller) usageError(err error) error { return a.env.WrapUsage(err) }

func (a controller) bindActionFlags(cmd *cobra.Command, opts *commandOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&opts.noProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (a controller) clientRequest(cmd *cobra.Command, opts commandOptions) (session.ProxyRequest, error) {
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return session.ProxyRequest{}, errors.New("use either --proxy or --no-proxy, not both")
	}
	req := session.ProxyRequest{}
	if noProxyChanged && opts.noProxy {
		empty := ""
		req.HTTPSProxyOverride = &empty
	} else if proxyChanged {
		req.HTTPSProxyOverride = &opts.proxy
	}
	return req, nil
}

func (a controller) newDownloadCommand() *cobra.Command {
	opts := downloadOptions{quality: string(download.QualityOriginal), ugoiraMode: string(download.UgoiraFormatGIF)}
	cmd := &cobra.Command{
		Use:   "download [SRC...]",
		Short: "Download illustrations",
		Args:  environment.ActionOrTargetsArgs(a.env.In, "pixiv download [options] SRC...", a.usageError),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDownload(cmd, args, opts)
		},
	}
	opts.input = environment.BindInput(cmd, environment.InputSpec{
		Codec:        environment.TextOrRecord,
		MinArgs:      1,
		MaxArgs:      -1,
		FillPosition: 0,
		Reader:       a.env.In,
		UsageError:   a.usageError,
	})
	a.bindActionFlags(cmd, &opts.commandOptions)
	a.bindDownloadRuntimeFlags(cmd, &opts)
	flags := cmd.Flags()
	flags.StringVar(&opts.pages, "pages", "", "1-based page selection, e.g. 1,3-5; default all pages")
	flags.StringVar(&opts.quality, "quality", opts.quality, "static image quality: original, regular, small, thumb, mini")
	flags.StringVar(&opts.ugoiraMode, "ugoira-mode", opts.ugoiraMode, "ugoira output mode: gif, apng")
	flags.StringVar(&opts.onError, "on-error", record.RecordFailureSkip, "record failure strategy: skip or fail-fast")
	environment.Bind(cmd, environment.Normal())
	return cmd
}

// bindDownloadRuntimeFlags 只注册真正影响下载落盘的参数，避免非下载命令静默接受无效 flag。
func (a controller) bindDownloadRuntimeFlags(cmd *cobra.Command, opts *downloadOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.downloadPath, "download-path", "", "download directory")
	flags.StringVarP(&opts.outputPath, "output", "o", "", "download directory (alias for --download-path)")
	flags.StringVar(&opts.filenameTemplate, "filename-template", "", "filename template placeholders: {id}, {title}, {author}, {author_id}, {date}, {tags}, {num}")
}

func (a controller) runDownload(cmd *cobra.Command, args []string, opts downloadOptions) error {
	if _, err := record.RecordFailureStrategy(opts.onError); err != nil {
		return a.usageError(err)
	}
	pages, err := saving.ParsePageSpec(opts.pages)
	if err != nil {
		return a.usageError(err)
	}
	quality := download.Quality(opts.quality)
	if quality == "" {
		quality = download.QualityOriginal
	}
	if err := saving.ValidateQuality(quality); err != nil {
		return a.usageError(err)
	}
	ugoiraFormat := download.UgoiraFormat(opts.ugoiraMode)
	if ugoiraFormat == "" {
		ugoiraFormat = download.UgoiraFormatGIF
	}
	if err := saving.ValidateFormat(ugoiraFormat); err != nil {
		return a.usageError(err)
	}
	clientReq, err := a.clientRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	runtime, err := a.runtime()
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("download-path") && cmd.Flags().Changed("output") && opts.downloadPath != opts.outputPath {
		return a.usageError(errors.New("--output and --download-path must name the same directory when both are provided"))
	}
	if cmd.Flags().Changed("output") {
		runtime.DownloadPath = opts.outputPath
	} else if cmd.Flags().Changed("download-path") {
		runtime.DownloadPath = opts.downloadPath
	}
	if cmd.Flags().Changed("filename-template") {
		runtime.FilenameTemplate = opts.filenameTemplate
	}
	request := saving.Request{Options: download.Options{
		DownloadPath:      runtime.DownloadPath,
		FilenameTemplate:  runtime.FilenameTemplate,
		DirectoryTemplate: runtime.DirectoryTemplate,
		Quality:           quality,
		UgoiraFormat:      ugoiraFormat,
	}, Pages: pages}
	// 先完成纯输入与运行参数校验，再取得账号和下载资源。
	downloadOne := func(ctx context.Context, id int64) error {
		return a.execute(ctx, clientReq, func(ctx context.Context, _ int64, client *pixiv.Client) (bool, error) {
			report, err := saving.Save(ctx, client, []string{strconv.FormatInt(id, 10)}, request)
			return downloadAttemptWithWarnings(a.env.Err, report, err)
		})
	}
	if len(args) == 0 {
		return environment.ConsumeActionRecords(cmd.Context(), opts.input.Reader(a.env.In), a.env.Err, "download", opts.onError, visualRecordTypes, downloadOne, a.usageError)
	}
	// 用户页与受资源策略允许的直链可能在一次调用里展开多个文件。只在结果中
	// 尚无已发布文件时，账号池才可因有效 Retry-After 安全重放这次调用。
	return a.execute(cmd.Context(), clientReq, func(ctx context.Context, _ int64, client *pixiv.Client) (bool, error) {
		report, err := saving.Save(ctx, client, args, request)
		return downloadAttemptWithWarnings(a.env.Err, report, err)
	})
}

// downloadAttemptResult 统一 action-record 与普通参数路径的提交边界：
// operation error 发生在部分结果之后时，仍以已发布文件阻止账号池重放。
func downloadAttemptResult(report saving.DownloadReport, operationErr error) (bool, error) {
	committed := ReportCommitted(report)
	if operationErr != nil {
		return committed, operationErr
	}
	return committed, ReportError(report)
}

// downloadAttemptWithWarnings 先保留 operation/business error，再附加 stderr
// 写入错误，避免 warning 输出故障掩盖真实下载失败。
func downloadAttemptWithWarnings(w io.Writer, report saving.DownloadReport, operationErr error) (bool, error) {
	committed, resultErr := downloadAttemptResult(report, operationErr)
	if warningErr := ReportWarnings(w, report); warningErr != nil {
		resultErr = errors.Join(resultErr, warningErr)
	}
	return committed, resultErr
}

// ReportWarnings 把下载过程中的非阻断提示写到 stderr，避免污染 stdout 的
// JSON/NDJSON 结果；warning 只包含下载层已分类的安全文本。
func ReportWarnings(w io.Writer, report saving.DownloadReport) error {
	for _, warning := range report.Warnings {
		label := "download"
		if warning.IllustID > 0 {
			label = fmt.Sprintf("artwork %d", warning.IllustID)
		}
		if warning.Type != "" {
			label += " (" + warning.Type + ")"
		}
		if _, err := fmt.Fprintf(w, "warning: %s: %s\n", label, warning.Message); err != nil {
			return err
		}
	}
	return nil
}

// ReportCommitted 只以已经原子发布的常规文件判断提交边界。资源获取或解析在此
// 之前失败时，账号池可识别有效 Retry-After 并切换未尝试账号。
func ReportCommitted(report saving.DownloadReport) bool {
	if report.Committed {
		return true
	}
	for _, item := range report.Items {
		if len(item.Files) > 0 {
			return true
		}
	}
	return false
}

// ReportError 把下载报告里的失败折叠成命令错误，并保留首个失败的分类错误，
// 使 shell 与账号池仍能区分 rate limit 等可分类原因。
func ReportError(report saving.DownloadReport) error {
	if len(report.Failures) == 0 {
		return nil
	}
	first := report.Failures[0]
	if first.Cause != nil {
		return fmt.Errorf("download completed with %d failures: %w", len(report.Failures), first.Cause)
	}
	if first.Message == "" {
		return fmt.Errorf("download completed with %d failures", len(report.Failures))
	}
	return fmt.Errorf("download completed with %d failures: %s", len(report.Failures), first.Message)
}
