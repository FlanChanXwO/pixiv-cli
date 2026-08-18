// Package download owns FANBOX post asset download commands.
package download

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/internal/listing"
	fanboxpost "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/post"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

type command struct {
	data deps.Data
	out  io.Writer
}

// New builds the actual `pixiv fanbox download` command.
func New(data deps.Data) *cobra.Command {
	a := command{data: data, out: data.Writer}
	cmd := &cobra.Command{
		Use:   "download SOURCE...",
		Short: "Download posts and their assets from FANBOX",
		Args:  data.RequireMinArgs(1, "pixiv fanbox download SOURCE..."),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args)
		},
	}
	data.BindTextValue(cmd, 1, -1, 0)
	requirements.Bind(cmd, requirements.FanboxData())
	return cmd
}

func (a command) run(cmd *cobra.Command, args []string) error {
	runtime, err := a.data.FanboxRuntimeConfig()
	if err != nil {
		return err
	}
	baseDir := filepath.Join(runtime.DownloadPath, "fanbox")
	return a.data.UseClient(cmd, func(client *fanbox.Client) error {
		seen := make(map[string]struct{})
		for _, source := range args {
			fetch, err := fanboxpost.PostsFetch(cmd.Context(), client, source)
			if err != nil {
				return err
			}
			if err := listing.Traverse(cmd.Context(), listing.AllResults(), fetch, func(posts []fanbox.Post) error {
				for _, post := range posts {
					if err := a.savePostAssets(cmd.Context(), client, baseDir, post, seen); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a command) savePostAssets(ctx context.Context, client *fanbox.Client, baseDir string, post fanbox.Post, seen map[string]struct{}) error {
	if post.Body == nil {
		return nil
	}
	dir := filepath.Join(baseDir, post.CreatorID, post.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, asset := range post.Body.Assets {
		if asset.Resource.Ref.IsZero() {
			continue
		}
		path := filepath.Join(dir, assetFilename(asset))
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		if _, err := client.SaveResource(ctx, asset.Resource.Ref, sdk.SaveOptions{Path: path}); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "saved: %s\n", path)
	}
	return nil
}

func assetFilename(asset fanbox.Asset) string {
	if asset.Name != "" {
		return asset.Name
	}
	ext := assetExtension(asset.Resource.URL)
	if asset.ID != "" {
		return asset.ID + ext
	}
	if ext != "" {
		return "asset" + ext
	}
	return "asset"
}

func assetExtension(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(parsed.Path))
	if len(ext) > 1 && len(ext) <= 12 {
		return ext
	}
	return ""
}
