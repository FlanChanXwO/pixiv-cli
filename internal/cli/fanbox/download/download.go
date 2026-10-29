// Package download 提供 pixiv fanbox download 命令，把 FANBOX 帖子资源保存到本地。
//
// 包的组成：[New] 组装命令。
// 资源打开与保存由 [github.com/FlanChanXwO/pixiv-cli/sdk/fanbox] 提供。
package download

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	fanboxcli "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/browse"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

type commandRunner struct {
	env          environment.Env
	open         func() (*session.Fanbox, error)
	downloadPath func() (string, error)
}

// New 构造 `pixiv fanbox download` 命令。downloadPath 只在命令执行时读取，
// 构造命令树不会打开配置文件。
func New(env environment.Env, open func() (*session.Fanbox, error), downloadPath func() (string, error)) *cobra.Command {
	a := commandRunner{env: env, open: open, downloadPath: downloadPath}
	cmd := &cobra.Command{
		Use:   "download SOURCE...",
		Short: "Download posts and their assets from FANBOX",
		Args:  env.MinArgs(1, "pixiv fanbox download SOURCE..."),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args)
		},
	}
	env.BindTextValue(cmd, 1, -1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) run(cmd *cobra.Command, args []string) error {
	downloadPath, err := a.downloadPath()
	if err != nil {
		return err
	}
	baseDir := filepath.Join(downloadPath, "fanbox")
	return fanboxcli.UseClient(cmd, a.open, func(client *fanbox.Client) error {
		seen := make(map[string]struct{})
		for _, source := range args {
			fetch, err := browse.PostsFetch(cmd.Context(), client, source)
			if err != nil {
				return err
			}
			if err := pagination.PageItems(cmd.Context(), pagination.PagePlan{}, func(ctx context.Context, cursor sdk.Cursor) ([]fanbox.Post, sdk.Cursor, error) {
				page, err := fetch(ctx, cursor)
				return page.Items, page.Next, err
			}, func(posts []fanbox.Post) error {
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

func (a commandRunner) savePostAssets(ctx context.Context, client *fanbox.Client, baseDir string, post fanbox.Post, seen map[string]struct{}) error {
	if post.Body == nil {
		return nil
	}
	// 每段路径都来自上游不可信内容（creator id、post id、asset name），必须单独
	// 清理并在 join 后校验最终绝对路径仍位于 baseDir 之内，杜绝 ../ 逃逸。
	creatorSeg := safePathSegment(post.CreatorID)
	postSeg := safePathSegment(post.ID)
	if creatorSeg == "" || postSeg == "" {
		return fmt.Errorf("post %s from creator %s has no usable path identity", post.ID, post.CreatorID)
	}
	dir := filepath.Join(baseDir, creatorSeg, postSeg)
	if !pathInsideBase(dir, baseDir) {
		return fmt.Errorf("post %s from creator %s resolves outside the download directory", post.ID, post.CreatorID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, asset := range post.Body.Assets {
		if asset.Resource.Ref.IsZero() {
			continue
		}
		name := safePathSegment(assetFilename(asset))
		if name == "" {
			return fmt.Errorf("asset %s in post %s has no usable filename", asset.ID, post.ID)
		}
		path := filepath.Join(dir, name)
		if !pathInsideBase(path, baseDir) {
			return fmt.Errorf("asset %s in post %s resolves outside the download directory", asset.ID, post.ID)
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		if _, err := client.SaveResource(ctx, asset.Resource.Ref, sdk.SaveOptions{Path: path}); err != nil {
			return err
		}
		fmt.Fprintf(a.env.Out, "saved: %s\n", path)
	}
	return nil
}

// safePathSegment 清理一个来自上游内容的路径分量。空段、`.`、`..` 和清理后仍含分隔符的
// 值都会被拒绝；调用方还必须确认拼接结果仍在基准目录内。
func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return ""
	}
	// 去掉单个文件名分量中绝不能出现的系统路径分隔符和 shell 元字符。
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, value)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return ""
	}
	return cleaned
}

// pathInsideBase 报告清理后的 target 是否等于 base 或位于其下。只做词法清理，
// 因此任何一段伪造的 `..` 都不能逃出下载目录。
func pathInsideBase(target, base string) bool {
	cleanTarget := filepath.Clean(target)
	cleanBase := filepath.Clean(base)
	if cleanTarget == cleanBase {
		return true
	}
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel)
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
