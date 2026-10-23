package artwork_media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// readAnimation reuses the CLI archive policy and Go encoder, but never
// publishes their temporary filesystem paths as the MCP delivery contract.
func readAnimation(ctx context.Context, client *pixiv.Client, id int64, format string) (content []mcp.Content, page outputs.MediaPage, err error) {
	generationStarted := time.Now()
	// 在清理之后记录最终结果，包含清理失败且不暴露临时路径。
	defer func() {
		count := 0
		if err == nil {
			count = page.Size
		}
		observeMediaPhase(ctx, "animation_generation", time.Since(generationStarted), count, err)
	}()
	if format == "" {
		format = "gif"
	}
	started := time.Now()
	meta, err := client.UgoiraMetadata(ctx, pixiv.UgoiraMetadataRequest{ArtworkID: id})
	observeMediaPhase(ctx, "animation_metadata", time.Since(started), 0, err)
	if err != nil {
		return nil, page, err
	}
	archive := downloader.SelectUgoiraArchive(meta)
	if archive == nil {
		return nil, page, sdk.NewError("pixiv", "artwork_media", sdk.ContentUnavailable)
	}
	dir, err := os.MkdirTemp("", "pixiv-mcp-ugoira-")
	if err != nil {
		return nil, page, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	zipPath := filepath.Join(dir, "frames.zip")
	started = time.Now()
	saved, err := client.SaveResource(ctx, archive.Resource.Ref, sdk.SaveOptions{Path: zipPath})
	observeMediaPhase(ctx, "animation_zip_download", time.Since(started), int(saved.Size), err)
	if err != nil {
		return nil, page, err
	}
	frames := make([]ugoira.Frame, len(meta.Frames))
	for i, frame := range meta.Frames {
		frames[i] = ugoira.Frame{File: frame.Filename, Delay: frame.DelayMilliseconds}
	}
	filename := fmt.Sprintf("%d.%s", id, format)
	animationPath := filepath.Join(dir, filename)
	var waitDuration, encodeDuration time.Duration
	var waitObserved, encodeObserved bool
	err = ugoira.NewEncoder().Encode(ctx, ugoira.Input{ZipPath: zipPath, Frames: frames, WorkDir: dir, OutputPath: animationPath, Format: ugoira.Format(format), Observe: func(stage string, duration time.Duration) {
		switch stage {
		case "animation_encode_wait":
			waitDuration, waitObserved = duration, true
		case "animation_encode":
			encodeDuration, encodeObserved = duration, true
		}
	}})
	if waitObserved {
		observeMediaPhase(ctx, "animation_encode_wait", waitDuration, 0, nil)
	}
	if encodeObserved {
		observeMediaPhase(ctx, "animation_encode", encodeDuration, 0, err)
	}
	if err != nil {
		return nil, page, err
	}
	started = time.Now()
	data, err := os.ReadFile(animationPath)
	observeMediaPhase(ctx, "animation_result_read", time.Since(started), len(data), err)
	if err != nil {
		return nil, page, err
	}
	decode := gif.Decode
	mimeType := "image/gif"
	if format == "apng" {
		decode = png.Decode
		mimeType = "image/apng"
	}
	first, err := decode(bytes.NewReader(data))
	if err != nil {
		return nil, page, err
	}
	var preview bytes.Buffer
	if err := png.Encode(&preview, first); err != nil {
		return nil, page, err
	}
	if err := ctx.Err(); err != nil {
		return nil, page, err
	}
	page = outputs.MediaPage{Page: 1, Filename: filename, MIMEType: mimeType, Size: len(data), ContentIndex: 0, PreviewContentIndex: new(1), ArchiveQuality: string(archive.Quality)}
	contentMIME := mimeType
	if format == "apng" {
		// PNG 家族类型让宿主保留完整 APNG；结构化信息仍明确动画格式。
		contentMIME = "image/png"
	}
	return []mcp.Content{&mcp.ImageContent{MIMEType: contentMIME, Data: data}, &mcp.ImageContent{MIMEType: "image/png", Data: preview.Bytes()}}, page, nil
}

// 阶段失败仅输出固定原因，保留业务错误通过原返回路径交付。
func observeMediaPhase(ctx context.Context, stage string, duration time.Duration, count int, err error) {
	event := logging.Event{Module: logging.ModulePixivMCP, Kind: logging.EventCompleted, Operation: stage, Duration: duration, Count: count}
	if err != nil {
		event.Kind = logging.EventFailed
		event.Reason = logging.ReasonToolFailed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			event.Reason = logging.ReasonRequestCancelled
		}
	}
	logging.Emit(ctx, event)
}
