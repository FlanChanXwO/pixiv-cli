// Package show_gallery owns explicit visual presentation (gallery and single-artwork detail), not search.
package show_gallery

import (
	"context"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type input struct {
	IllustIDs []int64 `json:"illust_ids"`
	Page      int     `json:"page,omitempty"`
	Quality   string  `json:"quality,omitempty"`
}
type image struct {
	IllustID  int64  `json:"illust_id"`
	Page      int    `json:"page"`
	URL       string `json:"url"`
	AvatarURL string `json:"avatar_url,omitempty"`
}
type failure struct {
	IllustID int64  `json:"illust_id"`
	Error    string `json:"error"`
}
type output struct {
	Records     []record.Record `json:"records"`
	Images      []image         `json:"images"`
	Failures    []failure       `json:"failures"`
	MediaOrigin string          `json:"media_origin,omitempty"`
}

func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "pixiv_show_gallery", Description: "Show a visual gallery ONLY when the user explicitly asks to see images or a gallery. Pass artwork IDs from existing results; does not repeat a search. Optional page and quality renew a selected image for the app; defaults to page 1 thumbnail. Do not call automatically after searches.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, OutputSchema: outputSchema(), InputSchema: schemas.ClosedObject(map[string]any{"illust_ids": map[string]any{"type": "array", "minItems": 1, "items": schemas.PositiveInteger("Artwork ID")}, "page": schemas.PositiveInteger("One-based page; defaults to 1"), "quality": schemas.EnumString("Defaults to thumbnail", "thumbnail", "regular", "original")}, []string{"illust_ids"})}, func(ctx context.Context, _ *mcp.CallToolRequest, in input) (*mcp.CallToolResult, output, error) {
		return handle(ctx, app, in)
	})
	toolhost.AddTool(app, server, &mcp.Tool{Name: "pixiv_show_artwork", Description: "Show one artwork's visual detail page ONLY when the user explicitly asks to see that artwork. Pass an artwork ID from existing results; returns the same result as pixiv_show_gallery with that single ID, which the app opens as a detail page. Do not call automatically after searches.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, OutputSchema: outputSchema(), InputSchema: schemas.ClosedObject(map[string]any{"illust_id": schemas.PositiveInteger("Artwork ID")}, []string{"illust_id"})}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		IllustID int64 `json:"illust_id"`
	}) (*mcp.CallToolResult, output, error) {
		return handle(ctx, app, input{IllustIDs: []int64{in.IllustID}})
	})
}
func handle(ctx context.Context, app *toolhost.App, in input) (*mcp.CallToolResult, output, error) {
	out := output{Records: []record.Record{}, Images: []image{}, Failures: []failure{}}
	finish := func() (*mcp.CallToolResult, output, error) {
		return records.Result(out.Records, len(out.Failures) > 0, "Gallery results; images load only in the explicit gallery."), out, nil
	}
	if len(in.IllustIDs) == 0 || in.Page < 0 {
		out.Failures = append(out.Failures, failure{Error: "invalid_request"})
		return finish()
	}
	for _, id := range in.IllustIDs {
		if id <= 0 {
			out.Failures = append(out.Failures, failure{Error: "invalid_request"})
			return finish()
		}
	}
	if in.Page == 0 {
		in.Page = 1
	}
	if in.Quality == "" {
		in.Quality = "thumbnail"
	}
	if in.Quality != "thumbnail" && in.Quality != "regular" && in.Quality != "original" {
		out.Failures = append(out.Failures, failure{Error: "invalid_request"})
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
	results := make([]output, len(ids))
	one := func(id int64) output {
		out := output{}
		artwork, err := app.Artwork(ctx, id)
		if err != nil {
			out.Failures = append(out.Failures, failure{id, "artwork_unavailable"})
			return out
		}
		rec, err := records.FromArtwork(artwork)
		if err != nil {
			out.Failures = append(out.Failures, failure{id, "invalid_metadata"})
			return out
		}
		out.Records = append(out.Records, rec)
		if artwork.Kind == pixiv.ArtworkKindUgoira && in.Quality == "original" {
			return out
		}
		media := app.SDKPorts().Media
		if media == nil {
			out.Failures = append(out.Failures, failure{id, "image_proxy_unavailable"})
			return out
		}
		out.MediaOrigin = media.Origin()
		account, err := app.ResolveAccount(ctx)
		if err != nil {
			out.Failures = append(out.Failures, failure{id, "account_unavailable"})
			return out
		}
		if in.Page > artwork.PageCount {
			out.Failures = append(out.Failures, failure{id, "page_unavailable"})
			return out
		}
		u, err := media.Issue(ctx, account.UserID, id, in.Page, in.Quality)
		if err != nil {
			out.Failures = append(out.Failures, failure{id, "image_authorization_unavailable"})
			return out
		}
		item := image{IllustID: id, Page: in.Page, URL: u}
		if !artwork.User.ProfileImage.Resource.Ref.IsZero() {
			item.AvatarURL, err = media.Issue(ctx, account.UserID, id, 1, "avatar")
			if err != nil {
				out.Failures = append(out.Failures, failure{id, "avatar_authorization_unavailable"})
			}
		}
		out.Images = append(out.Images, item)
		return out
	}
	workers := 4
	if media := app.SDKPorts().Media; media != nil {
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
			results[index].Failures = []failure{{ids[index], "artwork_unavailable"}}
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

func outputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[output](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["records"] = &jsonschema.Schema{Type: "array", Items: records.OpenObjectSchema()}
	return schema
}
