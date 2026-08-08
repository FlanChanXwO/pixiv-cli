package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	clioutput "github.com/FlanChanXwO/pixiv-cli/internal/cli/output"
	"github.com/spf13/cobra"
)

type controller struct {
	host   Host
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

type runtimeServices struct {
	SDK      pixivapp.SDKService
	Download downloadapp.DownloadService
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
	filenameTemplate string
	pages            string
	quality          string
	ugoiraMode       string
	onError          string
}

var visualRecordTypes = map[string]struct{}{
	"illust": {},
	"manga":  {},
	"ugoira": {},
}

func newController(host Host) controller {
	a := controller{host: host}
	if host != nil {
		a.in = host.Input()
		a.out = host.Output()
		a.errOut = host.ErrorOutput()
	}
	return a
}

func (a controller) services() runtimeServices {
	if a.host == nil {
		return runtimeServices{}
	}
	return runtimeServices{SDK: a.host.SDKService(), Download: a.host.DownloadService()}
}

func (a controller) usageError(err error) error {
	if err == nil {
		return nil
	}
	if a.host == nil {
		return err
	}
	return a.host.UsageError(err)
}

func (a controller) bindActionFlags(cmd *cobra.Command, opts *commandOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	flags.BoolVar(&opts.noProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (a controller) clientRequest(cmd *cobra.Command, opts commandOptions) (application.ClientRequest, error) {
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return application.ClientRequest{}, errors.New("use either --proxy or --no-proxy, not both")
	}
	req := application.ClientRequest{}
	if noProxyChanged && opts.noProxy {
		empty := ""
		req.HTTPSProxyOverride = &empty
	} else if proxyChanged {
		req.HTTPSProxyOverride = &opts.proxy
	}
	if flag := cmd.Flags().Lookup("sleep-request"); flag != nil && flag.Changed {
		value, err := time.ParseDuration(flag.Value.String())
		if err != nil {
			return application.ClientRequest{}, fmt.Errorf("invalid --sleep-request: %w", err)
		}
		if value < 0 {
			return application.ClientRequest{}, errors.New("--sleep-request must not be negative")
		}
		req.RequestIntervalOverride = &value
	}
	return req, nil
}

func (a controller) newDownloadCommand() *cobra.Command {
	opts := downloadOptions{quality: string(downloadapp.DownloadQualityOriginal), ugoiraMode: string(downloadapp.UgoiraFormatGIF)}
	cmd := &cobra.Command{
		Use:   "download [SRC...]",
		Short: "Download illustrations",
		Args:  clioutput.ActionOrTargetsArgs(a.in, "pixiv download [options] SRC...", a.usageError),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDownload(cmd, args, opts)
		},
	}
	a.bindActionFlags(cmd, &opts.commandOptions)
	a.bindDownloadRuntimeFlags(cmd, &opts)
	flags := cmd.Flags()
	flags.StringVar(&opts.pages, "pages", "", "1-based page selection, e.g. 1,3-5; default all pages")
	flags.StringVar(&opts.quality, "quality", opts.quality, "static image quality: original, regular, small, thumb, mini")
	flags.StringVar(&opts.ugoiraMode, "ugoira-mode", opts.ugoiraMode, "ugoira output mode: gif, apng")
	flags.StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	return cmd
}

// bindDownloadRuntimeFlags 只注册真正影响下载落盘的参数，避免非下载命令静默接受无效 flag。
func (a controller) bindDownloadRuntimeFlags(cmd *cobra.Command, opts *downloadOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.downloadPath, "download-path", "", "download directory")
	flags.StringVar(&opts.filenameTemplate, "filename-template", "", "filename template placeholders: {id}, {title}, {author}, {author_id}, {date}, {tags}, {num}")
}

func (a controller) runDownload(cmd *cobra.Command, args []string, opts downloadOptions) error {
	if _, err := clioutput.RecordFailureStrategy(opts.onError); err != nil {
		return a.usageError(err)
	}
	pages, err := downloadapp.ParsePageSpec(opts.pages)
	if err != nil {
		return a.usageError(err)
	}
	quality := downloadapp.DownloadQuality(opts.quality)
	if quality == "" {
		quality = downloadapp.DownloadQualityOriginal
	}
	if err := downloadapp.ValidateDownloadQuality(quality); err != nil {
		return a.usageError(err)
	}
	ugoiraFormat := downloadapp.UgoiraFormat(opts.ugoiraMode)
	if ugoiraFormat == "" {
		ugoiraFormat = downloadapp.UgoiraFormatGIF
	}
	if err := downloadapp.ValidateUgoiraFormat(ugoiraFormat); err != nil {
		return a.usageError(err)
	}
	services := a.services()
	clientReq, err := a.clientRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	runtime, err := services.SDK.Runtime()
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("download-path") {
		runtime.DownloadPath = opts.downloadPath
	}
	if cmd.Flags().Changed("filename-template") {
		runtime.FilenameTemplate = opts.filenameTemplate
	}
	request := downloadapp.DownloadRequest{
		DownloadPath:     runtime.DownloadPath,
		FilenameTemplate: runtime.FilenameTemplate,
		Pages:            pages,
		Quality:          quality,
		UgoiraFormat:     ugoiraFormat,
	}
	sdkRequest := pixivapp.SDKClientRequest{HTTPSProxyOverride: clientReq.HTTPSProxyOverride, RequestIntervalOverride: clientReq.RequestIntervalOverride}
	downloadOne := func(ctx context.Context, id int64) error {
		return services.SDK.RunPooledOperation(ctx, sdkRequest, func(ctx context.Context, client pixivapp.ClientSet) (bool, error) {
			report, err := services.Download.DownloadSources(ctx, client, []string{strconv.FormatInt(id, 10)}, request)
			if err != nil {
				return false, err
			}
			return downloadReportCommitted(report), downloadReportError(report)
		})
	}
	if len(args) == 0 {
		return clioutput.ConsumeActionRecords(cmd.Context(), a.in, a.errOut, "download", opts.onError, visualRecordTypes, downloadOne, a.usageError)
	}
	// 用户页与受资源策略允许的直链可能在一次调用里展开多个文件。只在结果中
	// 尚无已发布文件时，账号池才可因有效 Retry-After 安全重放这次调用。
	return services.SDK.RunPooledOperation(cmd.Context(), sdkRequest, func(ctx context.Context, client pixivapp.ClientSet) (bool, error) {
		report, err := services.Download.DownloadSources(ctx, client, args, request)
		if err != nil {
			return downloadReportCommitted(report), err
		}
		return downloadReportCommitted(report), downloadReportError(report)
	})
}

// downloadReportCommitted 只以已经原子发布的常规文件判断提交边界。资源获取
// 或解析在此之前失败时，账号池可识别有效 Retry-After 并切换未尝试账号。
func downloadReportCommitted(report downloadapp.DownloadReport) bool {
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

func downloadReportError(report downloadapp.DownloadReport) error {
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

// DownloadReportErrorForCLI、DownloadReportCommittedForCLI 是根 CLI 集成测试的
// 迁移 seam；实际下载命令只调用 package 内的稳定实现。
func DownloadReportErrorForCLI(report downloadapp.DownloadReport) error {
	return downloadReportError(report)
}

func DownloadReportCommittedForCLI(report downloadapp.DownloadReport) bool {
	return downloadReportCommitted(report)
}
