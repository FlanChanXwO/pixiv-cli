// Package download 提供 pixiv fanbox download 命令，把 FANBOX 帖子资源保存到本地。
//
// 包的组成：[New] 组装命令。
// 资源打开与保存由 [github.com/FlanChanXwO/pixiv-cli/sdk/fanbox] 提供。
package download

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	fanboxcli "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/browse"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/report"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

const postMetadataFilename = "post.json"

type savedAsset struct {
	id   string
	file os.FileInfo
}

type commandRunner struct {
	env          environment.Env
	open         fanboxcli.Opener
	downloadPath func() (string, error)
}

// New 构造 `pixiv fanbox download` 命令。downloadPath 只在命令执行时读取，
// 构造命令树不会打开配置文件。
func New(env environment.Env, open fanboxcli.Opener, downloadPath func() (string, error)) *cobra.Command {
	a := commandRunner{env: env, open: open, downloadPath: downloadPath}
	cmd := &cobra.Command{
		Use:   "download SOURCE...",
		Short: "help.download_posts_and_their_assets_from_fanbox",
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
		seen := make(map[string]savedAsset)
		for _, source := range args {
			fetch, err := browse.PostsFetch(cmd.Context(), client, source)
			if err != nil {
				return err
			}
			if err := pagination.PageItems(cmd.Context(), pagination.PagePlan{}, func(ctx context.Context, cursor sdk.Cursor) ([]*fanbox.Post, sdk.Cursor, error) {
				page, err := fetch(ctx, cursor)
				return page.Items, page.Cursor, err
			}, func(posts []*fanbox.Post) error {
				for _, post := range posts {
					if err := a.savePostAssets(cmd.Context(), baseDir, post, seen); err != nil {
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

func (a commandRunner) savePostAssets(ctx context.Context, baseDir string, post *fanbox.Post, seen map[string]savedAsset) error {
	if post.Body == nil && !post.IsRestricted {
		detail, err := post.Get(ctx)
		if err != nil {
			return report.Wrap(err, "Post")
		}
		post = detail
	}
	// 每段路径都来自上游不可信内容（creator id、post id、asset name），必须单独
	// 清理并在 join 后校验最终绝对路径仍位于 baseDir 之内，杜绝 ../ 逃逸。
	creatorSeg := safePathSegment(post.CreatorID)
	postSeg := safePathSegment(post.ID)
	if creatorSeg == "" || postSeg == "" {
		return i18n.Errorf("error.post_from_creator_has_no_usable_path_identity", post.ID, post.CreatorID)
	}
	dir := filepath.Join(baseDir, creatorSeg, postSeg)
	if !pathInsideBase(dir, baseDir) {
		return i18n.Errorf("error.post_from_creator_resolves_outside_the_download_directory", post.ID, post.CreatorID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	metadata, err := json.MarshalIndent(post, "", "  ")
	if err != nil {
		return err
	}
	if _, err := atomicfile.AtomicWrite(ctx, filepath.Join(dir, postMetadataFilename), bytes.NewReader(metadata)); err != nil {
		return err
	}
	if post.IsRestricted {
		return i18n.Errorf("error.post_is_restricted_metadata_saved_to", post.ID, dir)
	}
	if post.Body == nil {
		return i18n.Errorf("error.post_has_no_body_in_its_detail_response", post.ID)
	}
	save := func(id, name, locator string, write func(context.Context, string) error) error {
		name = safePathSegment(assetFilename(id, name, locator))
		if name == "" {
			return i18n.Errorf("error.asset_in_post_has_no_usable_filename", id, post.ID)
		}
		if strings.EqualFold(name, postMetadataFilename) {
			name = safePathSegment(id) + "_" + name
		}
		path := filepath.Join(dir, name)
		if !pathInsideBase(path, baseDir) {
			return i18n.Errorf("error.asset_in_post_resolves_outside_the_download_directory", id, post.ID)
		}
		if previous, exists := seen[path]; exists {
			if previous.id != id {
				return i18n.Errorf("error.assets_and_in_post_resolve_to_the_same", previous.id, id, post.ID)
			}
			return nil
		}
		if current, err := os.Stat(path); err == nil {
			for _, previous := range seen {
				if os.SameFile(current, previous.file) {
					if previous.id != id {
						return i18n.Errorf("error.assets_and_in_post_resolve_to_the_same", previous.id, id, post.ID)
					}
					return nil
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := write(ctx, path); err != nil {
			return report.Wrap(err, "OpenResource")
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		seen[path] = savedAsset{id: id, file: info}
		return nil
	}
	for _, image := range post.Body.Images {
		if err := save(image.ID, "", image.URL, image.Save); err != nil {
			return err
		}
	}
	for _, file := range post.Body.Files {
		if err := save(file.ID, file.Filename(), file.URL, file.Save); err != nil {
			return err
		}
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

func assetFilename(id, name, locator string) string {
	if name != "" {
		return name
	}
	ext := assetExtension(locator)
	if id != "" {
		return id + ext
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
