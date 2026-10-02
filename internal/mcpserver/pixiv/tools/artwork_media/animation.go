package artwork_media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// readAnimation reuses the CLI archive policy and native encoder, but never
// publishes their temporary filesystem paths as the MCP delivery contract.
func readAnimation(ctx context.Context, client *pixiv.Client, id int64, format string) (content []mcp.Content, page outputs.MediaPage, err error) {
	if format == "" {
		format = "gif"
	}
	meta, err := client.UgoiraMetadata(ctx, pixiv.UgoiraMetadataRequest{ArtworkID: id})
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
	if _, err := client.SaveResource(ctx, archive.Resource.Ref, sdk.SaveOptions{Path: zipPath}); err != nil {
		return nil, page, err
	}
	frames := make([]ugoira.Frame, len(meta.Frames))
	for i, frame := range meta.Frames {
		frames[i] = ugoira.Frame{File: frame.Filename, Delay: frame.DelayMilliseconds}
	}
	filename := fmt.Sprintf("%d.%s", id, format)
	animationPath := filepath.Join(dir, filename)
	if err := ugoira.NewRustEncoder().Encode(ctx, ugoira.Input{ZipPath: zipPath, Frames: frames, WorkDir: dir, OutputPath: animationPath, Format: ugoira.Format(format)}); err != nil {
		return nil, page, err
	}
	data, err := os.ReadFile(animationPath)
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
	return []mcp.Content{&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: fmt.Sprintf("urn:sha256:%x", sha256.Sum256(data)), MIMEType: mimeType, Blob: data}}, &mcp.ImageContent{MIMEType: "image/png", Data: preview.Bytes()}}, page, nil
}
