// Package artwork_media delivers artwork bytes rather than server-local files.
package artwork_media

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type input struct {
	IllustID        int64  `json:"illust_id" jsonschema:"Positive artwork ID"`
	Pages           []int  `json:"pages,omitempty" jsonschema:"One-based pages; omitted means all pages; duplicates retain first occurrence"`
	Quality         string `json:"quality,omitempty" jsonschema:"Static artwork only: regular (default), thumbnail, or original; omit for Ugoira"`
	AnimationFormat string `json:"animation_format,omitempty" jsonschema:"gif or apng; not applicable to static artwork"`
}

func Register(app *runtime.App, server *mcp.Server) {
	outputs.ProtectMediaInput(server, "pixiv_artwork_media")
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_artwork_media", Description: "Read artwork bytes: static images default to regular quality and all pages; Ugoira defaults to GIF with a separate PNG preview. Omit static pages/quality for Ugoira; use animation_format=apng for APNG. Use static original only when requested. Results map each delivered page to a content index and explicitly report partial failures. Does not save files on the server.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}}, func(ctx context.Context, _ *mcp.CallToolRequest, in input) (*mcp.CallToolResult, outputs.ArtworkMedia, error) {
		return handle(ctx, app, in)
	})
}

func handle(ctx context.Context, app *runtime.App, in input) (*mcp.CallToolResult, outputs.ArtworkMedia, error) {
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
	// One lease binds detail and every page to the same account; partial bytes are
	// never replayed with another account. Static reads stay in memory; the native
	// animation encoder owns a per-call temporary workspace that is removed before delivery.
	err := lifecycle.Run(ctx, app.OpenClient, func(ctx context.Context, client *pixiv.Client, _ *lifecycle.Attempt) error {
		artwork, err := client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: in.IllustID})
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
	return outputs.MediaResult(out, content)
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
