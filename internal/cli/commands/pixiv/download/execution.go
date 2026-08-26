package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type controller struct {
	deps   Deps
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

type runtimeServices struct {
	download downloader.DownloadService
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
	downloadPath     string
	outputPath       string
	filenameTemplate string
	pages            string
	quality          string
	ugoiraMode       string
	onError          string
}

var visualRecordTypes = map[string]struct{}{
	"artwork": {},
	"illust":  {},
	"manga":   {},
	"ugoira":  {},
}

func newController(deps Deps) controller {
	return controller{deps: deps, in: deps.Input, out: deps.Output, errOut: deps.ErrorOutput}
}

func (a controller) services() runtimeServices {
	services := runtimeServices{}
	if a.deps.Download != nil {
		services.download = a.deps.Download()
	}
	return services
}

func (a controller) usageError(err error) error {
	if err == nil {
		return nil
	}
	if a.deps.UsageError == nil {
		return err
	}
	return a.deps.UsageError(err)
}

func (a controller) bindActionFlags(cmd *cobra.Command, opts *commandOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&opts.noProxy, "no-proxy", false, "clear the configured proxy for this command")
}

// CommandRequest 是 download 命令解析 flags 后的本地请求值。
type CommandRequest struct {
	HTTPSProxyOverride *string
}

func (a controller) clientRequest(cmd *cobra.Command, opts commandOptions) (CommandRequest, error) {
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return CommandRequest{}, errors.New("use either --proxy or --no-proxy, not both")
	}
	req := CommandRequest{}
	if noProxyChanged && opts.noProxy {
		empty := ""
		req.HTTPSProxyOverride = &empty
	} else if proxyChanged {
		req.HTTPSProxyOverride = &opts.proxy
	}
	return req, nil
}

func (a controller) newDownloadCommand() *cobra.Command {
	opts := downloadOptions{quality: string(downloader.DownloadQualityOriginal), ugoiraMode: string(downloader.UgoiraFormatGIF)}
	cmd := &cobra.Command{
		Use:   "download [SRC...]",
		Short: "Download illustrations",
		Args:  pipeline.ActionOrTargetsArgs(a.in, "pixiv download [options] SRC...", a.usageError),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDownload(cmd, args, opts)
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{
		Codec:        pipeline.TextOrRecord,
		MinArgs:      1,
		MaxArgs:      -1,
		FillPosition: 0,
		Reader:       a.in,
		UsageError:   a.usageError,
	})
	a.bindActionFlags(cmd, &opts.commandOptions)
	a.bindDownloadRuntimeFlags(cmd, &opts)
	flags := cmd.Flags()
	flags.StringVar(&opts.pages, "pages", "", "1-based page selection, e.g. 1,3-5; default all pages")
	flags.StringVar(&opts.quality, "quality", opts.quality, "static image quality: original, regular, small, thumb, mini")
	flags.StringVar(&opts.ugoiraMode, "ugoira-mode", opts.ugoiraMode, "ugoira output mode: gif, apng")
	flags.StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	requirements.Bind(cmd, requirements.DownloadCommand())
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
	if _, err := pipeline.RecordFailureStrategy(opts.onError); err != nil {
		return a.usageError(err)
	}
	pages, err := downloader.ParsePageSpec(opts.pages)
	if err != nil {
		return a.usageError(err)
	}
	quality := downloader.DownloadQuality(opts.quality)
	if quality == "" {
		quality = downloader.DownloadQualityOriginal
	}
	if err := downloader.ValidateDownloadQuality(quality); err != nil {
		return a.usageError(err)
	}
	ugoiraFormat := downloader.UgoiraFormat(opts.ugoiraMode)
	if ugoiraFormat == "" {
		ugoiraFormat = downloader.UgoiraFormatGIF
	}
	if err := downloader.ValidateUgoiraFormat(ugoiraFormat); err != nil {
		return a.usageError(err)
	}
	clientReq, err := a.clientRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	runtime, err := a.deps.Runtime()
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
	request := downloader.DownloadRequest{
		DownloadPath:      runtime.DownloadPath,
		FilenameTemplate:  runtime.FilenameTemplate,
		DirectoryTemplate: runtime.DirectoryTemplate,
		Pages:             pages,
		Quality:           quality,
		UgoiraFormat:      ugoiraFormat,
	}
	// 先完成所有纯输入和运行参数校验，再解析下载服务。这样 owner 的依赖端口
	// 不会在互斥 flag 等用户输入错误上提前触发资源或文件系统准备。
	services := a.services()
	downloadOne := func(ctx context.Context, id int64) error {
		return a.deps.Pooled(ctx, clientReq, func(ctx context.Context, client *pixiv.Client) (bool, error) {
			report, err := services.download.DownloadSources(ctx, client, []string{strconv.FormatInt(id, 10)}, request)
			if err != nil {
				return false, err
			}
			return ReportCommitted(report), ReportError(report)
		})
	}
	if len(args) == 0 {
		return pipeline.ConsumeActionRecords(cmd.Context(), pipeline.Reader(cmd, a.in), a.errOut, "download", opts.onError, visualRecordTypes, downloadOne, a.usageError)
	}
	// 用户页与受资源策略允许的直链可能在一次调用里展开多个文件。只在结果中
	// 尚无已发布文件时，账号池才可因有效 Retry-After 安全重放这次调用。
	return a.deps.Pooled(cmd.Context(), clientReq, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		report, err := services.download.DownloadSources(ctx, client, args, request)
		if err != nil {
			return ReportCommitted(report), err
		}
		return ReportCommitted(report), ReportError(report)
	})
}

// ReportCommitted 只以已经原子发布的常规文件判断提交边界。资源获取或解析在此
// 之前失败时，账号池可识别有效 Retry-After 并切换未尝试账号。
func ReportCommitted(report downloader.DownloadReport) bool {
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

// ReportError 把下载报告里的失败折叠成命令错误，并保留首个失败的 typed cause，
// 使 shell 与账号池仍能区分 rate limit 等可分类原因。
func ReportError(report downloader.DownloadReport) error {
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
