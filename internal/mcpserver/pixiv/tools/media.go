// pixiv media tools.

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"image/gif"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
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
	attrs := []slog.Attr{
		slog.String(logging.AttrStage, stage),
		slog.Duration(logging.AttrDuration, duration),
		slog.Int(logging.AttrCount, count),
	}
	if err != nil {
		reason := "tool failed"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			reason = "request cancelled"
		}
		logging.From(ctx).LogAttrs(ctx, slog.LevelWarn, "Media step failed", append(attrs, slog.String(logging.AttrReason, reason))...)
		return
	}
	logging.From(ctx).LogAttrs(ctx, slog.LevelDebug, "Media step completed", attrs...)
}

type artworkMediaIn struct {
	IllustID        int64  `json:"illust_id" jsonschema:"Positive artwork ID"`
	Pages           []int  `json:"pages,omitempty" jsonschema:"One-based pages; omitted means all pages; duplicates retain first occurrence"`
	Quality         string `json:"quality,omitempty" jsonschema:"Static artwork only: regular (default), thumbnail, or original; omit for Ugoira"`
	AnimationFormat string `json:"animation_format,omitempty" jsonschema:"gif or apng; not applicable to static artwork"`
}

func (t pixivTools) registerArtworkMedia(server *mcp.Server) {
	outputs.ProtectMediaInput(server, "pixiv_artwork_media")
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_artwork_media", Description: "Read artwork bytes: static images default to regular quality and all pages; Ugoira defaults to GIF with a separate PNG preview. Omit static pages/quality for Ugoira; use animation_format=apng for APNG. Use static original only when requested. Results map each delivered page to a content index and explicitly report partial failures. Returns complete media bytes; the App view may offer protected temporary downloads.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}}, func(ctx context.Context, request *mcp.CallToolRequest, in artworkMediaIn) (*mcp.CallToolResult, outputs.ArtworkMedia, error) {
		result, out, err := t.artworkMediaHandler(ctx, in)
		appAnimationContent(request, result, out)
		return result, out, err
	})
}

func appAnimationContent(request *mcp.CallToolRequest, result *mcp.CallToolResult, out outputs.ArtworkMedia) {
	if result == nil || len(out.Pages) != 1 || out.Pages[0].PreviewContentIndex == nil {
		return
	}
	caps := request.ClientCapabilities()
	if caps == nil {
		return
	}
	var ui struct {
		MIMETypes []string `json:"mimeTypes"`
	}
	raw, err := json.Marshal(caps.Extensions["io.modelcontextprotocol/ui"])
	if err != nil || json.Unmarshal(raw, &ui) != nil || !slices.Contains(ui.MIMETypes, "text/html;profile=mcp-app") {
		return
	}
	// App 优先读取已发布的原质量文件，避免大控制响应阻塞宿主交付。
	if result.Meta == nil {
		result.Meta = mcp.Meta{}
	}
	contents := slices.Clone(result.Content)
	page := out.Pages[0]
	downloads, _ := result.Meta["pixiv/mediaDownloads"].(map[string]any)
	pages, _ := downloads["pages"].([]map[string]any)
	if len(pages) == 0 {
		return
	}
	for _, download := range pages {
		if url, ok := download["url"].(string); ok && url != "" && download["page"] == page.Page {
			contents[page.ContentIndex] = &mcp.ResourceLink{URI: url, Name: page.Filename, MIMEType: page.MIMEType, Size: new(int64(page.Size))}
			break
		}
	}
	result.Meta["pixiv/mediaContents"] = contents
	result.Content = []mcp.Content{result.Content[*out.Pages[0].PreviewContentIndex]}
}

func (t pixivTools) artworkMediaHandler(ctx context.Context, in artworkMediaIn) (*mcp.CallToolResult, outputs.ArtworkMedia, error) {
	out := outputs.NewArtworkMedia(in.IllustID)
	requestedQuality := in.Quality
	if in.Quality == "" {
		in.Quality = "regular"
	}
	switch {
	case in.IllustID <= 0:
		out.Error = "invalid_illust_id"
	case in.Quality != "regular" && in.Quality != "thumbnail" && in.Quality != "original":
		out.Error = "invalid_quality"
	case in.AnimationFormat != "" && in.AnimationFormat != "gif" && in.AnimationFormat != "apng":
		out.Error = "invalid_animation_format"
	case in.Pages != nil && len(in.Pages) == 0:
		out.Error = "invalid_pages"
	}
	seen := map[int]bool{}
	for _, p := range in.Pages {
		if p <= 0 {
			out.Error = "invalid_pages"
		}
		if !seen[p] {
			out.RequestedPages = append(out.RequestedPages, p)
			seen[p] = true
		}
	}
	if out.Error != "" {
		return outputs.MediaResult(out, nil)
	}
	var content []mcp.Content
	var source *pixiv.Client
	// One lease binds detail and every page to the same account; partial bytes are
	// never replayed with another account. Static reads stay in memory; the native
	// animation encoder owns a per-call temporary workspace that is removed before delivery.
	err := lifecycle.Run(ctx, t.app.OpenClient, func(ctx context.Context, client *pixiv.Client, _ *lifecycle.Attempt) error {
		source = client
		started := time.Now()
		artwork, err := client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: in.IllustID})
		observeMediaPhase(ctx, "artwork_query", time.Since(started), 0, err)
		if err != nil {
			return err
		}
		out.Title = artwork.Title
		out.TotalPages = artwork.PageCount
		if artwork.Kind == pixiv.ArtworkKindUgoira {
			if in.Pages != nil || requestedQuality != "" {
				out.Error = "static_parameters_not_applicable"
				return nil
			}
			out.TotalPages = 1
			out.RequestedPages = []int{1}
			media, page, err := readAnimation(ctx, client, in.IllustID, in.AnimationFormat)
			if err != nil {
				out.Failures = append(out.Failures, outputs.MediaFailure{Page: 1, Error: errorCode(err)})
				return err
			}
			content = media
			out.Pages = append(out.Pages, page)
			out.DeliveredPages = []int{1}
			return nil
		}
		if in.AnimationFormat != "" {
			out.Error = "animation_format_not_applicable"
			return nil
		}
		if out.TotalPages <= 0 {
			out.Error = "invalid_page_metadata"
			return nil
		}
		if in.Pages == nil {
			for p := 1; p <= out.TotalPages; p++ {
				out.RequestedPages = append(out.RequestedPages, p)
			}
		}
		for _, p := range out.RequestedPages {
			if p > out.TotalPages {
				out.Error = "invalid_pages"
				return nil
			}
		}
		pages := make(map[int]pixiv.ArtworkPage, len(artwork.Pages))
		for _, page := range artwork.Pages {
			pages[page.PageIndex+1] = page
		}
		variant := in.Quality
		if variant == "thumbnail" {
			variant = "thumb"
		}
		for _, p := range out.RequestedPages {
			failure := outputs.MediaFailure{Page: p}
			if err := ctx.Err(); err != nil {
				failure.Error = errorCode(err)
			} else if page, ok := pages[p]; !ok {
				failure.Error = "page_unavailable"
			} else {
				data, mime, status, err := readPage(ctx, client, page, variant)
				if err != nil {
					failure.Error = errorCode(err)
					failure.HTTPStatus = status
				} else {
					out.Pages = append(out.Pages, outputs.MediaPage{Page: p, MIMEType: mime, Size: len(data), ContentIndex: len(content)})
					content = append(content, &mcp.ImageContent{Data: data, MIMEType: mime})
					out.DeliveredPages = append(out.DeliveredPages, p)
					continue
				}
			}
			out.Failures = append(out.Failures, failure)
		}
		return ctx.Err()
	})
	if err != nil {
		out.Error = errorCode(err)
	}
	out.Complete = out.Error == "" && len(out.RequestedPages) > 0 && len(out.DeliveredPages) == len(out.RequestedPages)
	result, output, resultErr := outputs.MediaResult(out, content)
	if media := t.app.SDKPorts().Media; media != nil && len(out.Pages) > 0 && ctx.Err() == nil {
		account, accountErr := t.app.ResolveAccount(ctx)
		if accountErr == nil {
			downloads := []map[string]any{}
			for _, page := range out.Pages {
				var url string
				if page.PreviewContentIndex != nil {
					original := content[page.ContentIndex].(*mcp.ImageContent)
					preview := content[*page.PreviewContentIndex].(*mcp.ImageContent)
					format := strings.TrimPrefix(page.MIMEType, "image/")
					published, publishErr := media.PublishAnimation(ctx, account.UserID, in.IllustID, format, source, original.Data, preview.Data)
					if publishErr == nil {
						url = published.AnimationURL
					}
				} else if in.Quality == "original" {
					url, _ = media.Issue(ctx, account.UserID, in.IllustID, page.Page, "original")
				}
				if url != "" {
					downloads = append(downloads, map[string]any{"page": page.Page, "url": url})
				}
			}
			result.Meta = mcp.Meta{"pixiv/mediaDownloads": map[string]any{"media_origin": media.Origin(), "pages": downloads}}
		}
	}
	return result, output, resultErr
}

func readPage(ctx context.Context, client *pixiv.Client, page pixiv.ArtworkPage, variant string) (data []byte, mime string, status int, err error) {
	ref, err := pixiv.ArtworkVariantResource(page.Image.Resource, variant)
	if err != nil {
		return nil, "", 0, err
	}
	response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: ref})
	if err != nil {
		return nil, "", 0, err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return nil, "", response.StatusCode, sdk.NewError("pixiv", "artwork_media", sdk.UpstreamError)
	}
	data, err = io.ReadAll(response.Body)
	if err != nil {
		return nil, "", 0, err
	}
	// Inspect bytes, not a possibly incorrect upstream header or URL extension.
	mime = http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", 0, sdk.NewError("pixiv", "artwork_media", sdk.MalformedUpstreamResponse)
	}
	return data, mime, 0, nil
}

func errorCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	var typed *sdk.Error
	if errors.As(err, &typed) {
		return string(typed.Reason)
	}
	return "media_read_failed"
}

type galleryAnimationIn struct {
	Action   string `json:"action"`
	IllustID int64  `json:"illust_id,omitempty"`
	Format   string `json:"format,omitempty"`
	Handle   string `json:"handle,omitempty"`
}

func (t pixivTools) registerGalleryAnimation(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_gallery_animation", Description: "Prepare, inspect or release a Gallery animation subscription. Short control responses contain protected animation and PNG preview URLs when ready. Poll status every 2 seconds; failures require explicit retry. Existing pixiv_artwork_media delivers full embedded media.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), OpenWorldHint: new(true)}, InputSchema: schemas.ClosedObject(map[string]any{"action": schemas.EnumString("Operation", "prepare", "status", "release"), "illust_id": schemas.PositiveInteger("Artwork ID for prepare"), "format": schemas.EnumString("Prepare format, default gif", "gif", "apng"), "handle": map[string]any{"type": "string", "minLength": 1}}, []string{"action"})}, func(ctx context.Context, _ *mcp.CallToolRequest, in galleryAnimationIn) (*mcp.CallToolResult, mediaproxy.AnimationStatus, error) {
		out := mediaproxy.AnimationStatus{State: "failed"}
		fail := func(code string) (*mcp.CallToolResult, mediaproxy.AnimationStatus, error) {
			out.State = "failed"
			out.Error = code
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: code}}}, out, nil
		}
		if in.Action == "prepare" {
			if in.IllustID <= 0 || in.Handle != "" || (in.Format != "" && in.Format != "gif" && in.Format != "apng") {
				return fail("invalid_request")
			}
		} else if (in.Action != "status" && in.Action != "release") || in.Handle == "" || in.IllustID != 0 || in.Format != "" {
			return fail("invalid_request")
		}
		media := t.app.SDKPorts().Media
		if media == nil {
			return fail("animation_service_unavailable")
		}
		account, err := t.app.ResolveAccount(ctx)
		if err != nil {
			return fail("account_unavailable")
		}
		if in.Action == "prepare" {
			out, err = media.PrepareAnimation(ctx, account.UserID, in.IllustID, in.Format)
		} else {
			out, err = media.AnimationSubscription(ctx, account.UserID, in.Handle, in.Action == "release")
		}
		if err != nil {
			return fail("animation_unavailable")
		}
		var meta mcp.Meta
		if out.State == "ready" && out.FrameCount > 0 {
			meta = mcp.Meta{"pixiv/animationFrameCount": out.FrameCount}
		}
		return &mcp.CallToolResult{Meta: meta, IsError: out.State == "failed", Content: []mcp.Content{&mcp.TextContent{Text: "Animation " + out.State}}}, out, nil
	})
}

type showGalleryIn struct {
	IllustIDs []int64 `json:"illust_ids"`
	Page      int     `json:"page,omitempty"`
	Pages     []int   `json:"pages,omitempty"`
	Quality   string  `json:"quality,omitempty"`
}
type galleryImage struct {
	IllustID  int64  `json:"illust_id"`
	Page      int    `json:"page"`
	URL       string `json:"url"`
	AvatarURL string `json:"avatar_url,omitempty"`
}
type galleryFailure struct {
	IllustID int64  `json:"illust_id"`
	Error    string `json:"error"`
	// Page names the failed page of a multi-page request; artwork-level failures omit it.
	Page int `json:"page,omitempty"`
}
type galleryOut struct {
	Records     []record.Record  `json:"records"`
	Images      []galleryImage   `json:"images"`
	Failures    []galleryFailure `json:"failures"`
	MediaOrigin string           `json:"media_origin,omitempty"`
}

func (t pixivTools) registerShowGallery(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_show_gallery", Description: "Show a visual gallery ONLY when the user explicitly asks to see images or a gallery. Pass artwork IDs from existing results; does not repeat a search. Optional page (or pages, not both) and quality renew selected images for the app; defaults to page 1 thumbnail. Do not call automatically after searches.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, OutputSchema: showGalleryOutputSchema(), InputSchema: schemas.ClosedObject(map[string]any{"illust_ids": map[string]any{"type": "array", "minItems": 1, "items": schemas.PositiveInteger("Artwork ID")}, "page": schemas.PositiveInteger("One-based page; defaults to 1"), "pages": map[string]any{"type": "array", "minItems": 1, "items": schemas.PositiveInteger("One-based page"), "description": "Several one-based pages signed in one call; excludes page"}, "quality": schemas.EnumString("Defaults to thumbnail", "thumbnail", "regular", "original")}, []string{"illust_ids"})}, func(ctx context.Context, _ *mcp.CallToolRequest, in showGalleryIn) (*mcp.CallToolResult, galleryOut, error) {
		return t.showGalleryHandler(ctx, in)
	})
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_show_artwork", Description: "Show one artwork's visual detail page ONLY when the user explicitly asks to see that artwork. Pass an artwork ID from existing results; returns the same result as pixiv_show_gallery with that single ID, which the app opens as a detail page. Do not call automatically after searches.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, OutputSchema: showGalleryOutputSchema(), InputSchema: schemas.ClosedObject(map[string]any{"illust_id": schemas.PositiveInteger("Artwork ID")}, []string{"illust_id"})}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		IllustID int64 `json:"illust_id"`
	}) (*mcp.CallToolResult, galleryOut, error) {
		return t.showGalleryHandler(ctx, showGalleryIn{IllustIDs: []int64{in.IllustID}})
	})
}
func (t pixivTools) showGalleryHandler(ctx context.Context, in showGalleryIn) (*mcp.CallToolResult, galleryOut, error) {
	out := galleryOut{Records: []record.Record{}, Images: []galleryImage{}, Failures: []galleryFailure{}}
	finish := func() (*mcp.CallToolResult, galleryOut, error) {
		return records.Result(out.Records, len(out.Failures) > 0, "Gallery results; images load only in the explicit gallery."), out, nil
	}
	if len(in.IllustIDs) == 0 || in.Page < 0 || (len(in.Pages) > 0 && in.Page != 0) {
		out.Failures = append(out.Failures, galleryFailure{Error: "invalid_request"})
		return finish()
	}
	for _, id := range in.IllustIDs {
		if id <= 0 {
			out.Failures = append(out.Failures, galleryFailure{Error: "invalid_request"})
			return finish()
		}
	}
	if in.Page == 0 {
		in.Page = 1
	}
	// A single page is the one-element case; repeated pages keep their first position.
	pages := []int{in.Page}
	if len(in.Pages) > 0 {
		pages = pages[:0]
		seenPage := map[int]bool{}
		for _, page := range in.Pages {
			if page <= 0 {
				out.Failures = append(out.Failures, galleryFailure{Error: "invalid_request"})
				return finish()
			}
			if !seenPage[page] {
				seenPage[page] = true
				pages = append(pages, page)
			}
		}
	}
	if in.Quality == "" {
		in.Quality = "thumbnail"
	}
	if in.Quality != "thumbnail" && in.Quality != "regular" && in.Quality != "original" {
		out.Failures = append(out.Failures, galleryFailure{Error: "invalid_request"})
		return finish()
	}

	ids := make([]int64, 0, len(in.IllustIDs))
	seen := map[int64]bool{}
	for _, id := range in.IllustIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	results := make([]galleryOut, len(ids))
	one := func(id int64) galleryOut {
		out := galleryOut{}
		artwork, err := t.app.Artwork(ctx, id)
		if err != nil {
			out.Failures = append(out.Failures, galleryFailure{IllustID: id, Error: "artwork_unavailable"})
			return out
		}
		rec, err := record.RecordFromArtwork(artwork)
		if err != nil {
			out.Failures = append(out.Failures, galleryFailure{IllustID: id, Error: "invalid_metadata"})
			return out
		}
		out.Records = append(out.Records, rec)
		if artwork.Kind == pixiv.ArtworkKindUgoira && in.Quality == "original" {
			return out
		}
		media := t.app.SDKPorts().Media
		if media == nil {
			out.Failures = append(out.Failures, galleryFailure{IllustID: id, Error: "image_proxy_unavailable"})
			return out
		}
		out.MediaOrigin = media.Origin()
		account, err := t.app.ResolveAccount(ctx)
		if err != nil {
			out.Failures = append(out.Failures, galleryFailure{IllustID: id, Error: "account_unavailable"})
			return out
		}
		avatar := !artwork.User.ProfileImage.Resource.Ref.IsZero()
		for _, page := range pages {
			if page > artwork.PageCount {
				out.Failures = append(out.Failures, galleryFailure{IllustID: id, Error: "page_unavailable", Page: page})
				continue
			}
			u, err := media.Issue(ctx, account.UserID, id, page, in.Quality)
			if err != nil {
				out.Failures = append(out.Failures, galleryFailure{IllustID: id, Error: "image_authorization_unavailable", Page: page})
				continue
			}
			item := galleryImage{IllustID: id, Page: page, URL: u}
			// The avatar belongs to the artwork, so only its first delivered page carries it.
			if avatar {
				avatar = false
				item.AvatarURL, err = media.Issue(ctx, account.UserID, id, 1, "avatar")
				if err != nil {
					out.Failures = append(out.Failures, galleryFailure{IllustID: id, Error: "avatar_authorization_unavailable"})
				}
			}
			out.Images = append(out.Images, item)
		}
		return out
	}
	workers := 4
	if media := t.app.SDKPorts().Media; media != nil {
		workers = media.ConcurrentReads()
	}
	if workers > len(ids) {
		workers = len(ids)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index] = one(ids[index])
			}
		}()
	}
	for index := range ids {
		select {
		case jobs <- index:
		case <-ctx.Done():
			results[index].Failures = []galleryFailure{{IllustID: ids[index], Error: "artwork_unavailable"}}
		}
	}
	close(jobs)
	wg.Wait()
	for _, result := range results {
		out.Records = append(out.Records, result.Records...)
		out.Images = append(out.Images, result.Images...)
		out.Failures = append(out.Failures, result.Failures...)
		if result.MediaOrigin != "" {
			out.MediaOrigin = result.MediaOrigin
		}
	}

	return finish()
}

func showGalleryOutputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[galleryOut](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["records"] = &jsonschema.Schema{Type: "array", Items: records.OpenObjectSchema()}
	return schema
}
