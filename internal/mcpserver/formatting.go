package mcpserver

import (
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func formatIllusts(illusts []sdk.Illust, includeThumbnail bool, offset int, ranked bool) string {
	lines := make([]string, 0, len(illusts))
	for i, illust := range illusts {
		prefix := ""
		if ranked {
			prefix = fmt.Sprintf("第 %d 名: ", i+1+offset)
		}
		lines = append(lines, prefix+formatIllust(illust, includeThumbnail))
	}
	return strings.Join(lines, "\n\n")
}

func formatIllust(illust sdk.Illust, includeThumbnail bool) string {
	tags := make([]string, 0, len(illust.Tags))
	for _, tag := range illust.Tags {
		tags = append(tags, tag.Name)
	}
	text := fmt.Sprintf("ID: %d - %q\n  作者: %s (ID: %d)\n  类型: %s\n  标签: %s\n  收藏数: %d, 浏览数: %d",
		illust.ID, illust.Title, illust.User.Name, illust.User.ID, illust.Type, strings.Join(tags, ", "), illust.TotalBookmarks, illust.TotalView)
	if includeThumbnail {
		if url := thumbnailURL(illust); url != "" {
			text += "\n  缩略图: " + url
		} else {
			text += "\n  缩略图: 暂无"
		}
	}
	return text
}

func formatUsers(users []sdk.UserPreview) string {
	lines := make([]string, 0, len(users))
	for _, preview := range users {
		user := preview.User
		followed := "未关注"
		if user.IsFollowed {
			followed = "已关注"
		}
		comment := user.Comment
		if comment == "" {
			comment = "无"
		}
		lines = append(lines, fmt.Sprintf("用户ID: %d - %s (@%s)\n  关注状态: %s\n  简介: %s", user.ID, user.Name, user.Account, followed, comment))
	}
	return strings.Join(lines, "\n\n")
}

// formatSDKIllusts/formatSDKUsers 延续旧 MCP 中文文本结果，同时 structured content 提供
// 面向调用方的规范化 SDK 模型。
func formatSDKIllusts(illusts []sdk.Illust) string {
	lines := make([]string, 0, len(illusts))
	for _, illust := range illusts {
		tags := make([]string, 0, len(illust.Tags))
		for _, tag := range illust.Tags {
			tags = append(tags, tag.Name)
		}
		lines = append(lines, fmt.Sprintf("ID: %d - %q\n  作者: %s (ID: %d)\n  类型: %s\n  标签: %s\n  收藏数: %d, 浏览数: %d",
			illust.ID, illust.Title, illust.User.Name, illust.User.ID, illust.Type, strings.Join(tags, ", "), illust.TotalBookmarks, illust.TotalView))
	}
	return strings.Join(lines, "\n\n")
}

func formatSDKUsers(users []sdk.UserPreview) string {
	lines := make([]string, 0, len(users))
	for _, preview := range users {
		user := preview.User
		followed := "未关注"
		if user.IsFollowed {
			followed = "已关注"
		}
		comment := user.Comment
		if comment == "" {
			comment = "无"
		}
		lines = append(lines, fmt.Sprintf("用户ID: %d - %s (@%s)\n  关注状态: %s\n  简介: %s", user.ID, user.Name, user.Account, followed, comment))
	}
	return strings.Join(lines, "\n\n")
}

func thumbnailURL(illust sdk.Illust) string {
	for _, value := range []string{illust.ImageURLs.SquareMedium, illust.ImageURLs.Medium} {
		if value != "" {
			return value
		}
	}
	if len(illust.MetaPages) > 0 {
		return text.FirstNonEmpty(illust.MetaPages[0].ImageURLs.SquareMedium, illust.MetaPages[0].ImageURLs.Medium)
	}
	return text.FirstNonEmpty(illust.MetaSinglePage.OriginalImageURL, illust.ImageURLs.Large)
}

type stringWriter struct {
	builder *strings.Builder
}

func (w stringWriter) Write(p []byte) (int, error) {
	return w.builder.WriteString(string(p))
}
