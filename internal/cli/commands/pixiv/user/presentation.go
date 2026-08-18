package user

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func printArtworks(out io.Writer, items []pixiv.Artwork) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "https://www.pixiv.net/artworks/%d\n", item.ID); err != nil {
			return err
		}
		tags := make([]string, 0, len(item.Tags))
		for _, tag := range item.Tags {
			tags = append(tags, tag.Name)
		}
		if _, err := fmt.Fprintf(out, "%d %q by %s bookmarks:%d views:%d tags:%s\n", item.ID, item.Title, item.User.Name, item.TotalBookmarks, item.TotalViews, strings.Join(tags, ",")); err != nil {
			return err
		}
	}
	return nil
}

func printNovels(out io.Writer, items []pixiv.Novel) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "%d %s — %s\n", item.ID, item.Title, item.User.Name); err != nil {
			return err
		}
	}
	return nil
}

func printUserPreviews(out io.Writer, users []pixiv.UserPreview) error {
	for _, item := range users {
		if _, err := fmt.Fprintf(out, "%d %s\n", item.User.ID, item.User.Name); err != nil {
			return err
		}
	}
	return nil
}

// printUserSearchPreviews 展示搜索结果里的账号与简介，逐行转义上游文本，避免控制
// 字节破坏终端的逐行协议。
func printUserSearchPreviews(out io.Writer, items []pixiv.UserPreview) error {
	for _, item := range items {
		line := fmt.Sprintf("%d %s", item.User.ID, text.SafeLine(item.User.Name))
		if item.User.Account != "" {
			line += " (@" + text.SafeLine(item.User.Account) + ")"
		}
		if item.User.Comment != "" {
			line += " — " + text.SafeLine(item.User.Comment)
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

// printUserDetail 只展示人可读且有值的文本字段；固定计数保留零值，以免把公开的
// “没有作品/关注”误报为字段缺失。完整机器可读字段由 --json 原样输出 SDK DTO。
func printUserDetail(out io.Writer, result pixiv.UserDetail) error {
	lines := []string{fmt.Sprintf("user id: %d", result.User.ID)}
	if result.User.Name != "" {
		lines = append(lines, fmt.Sprintf("name: %s", result.User.Name))
	}
	if result.User.Account != "" {
		lines = append(lines, fmt.Sprintf("account: %s", result.User.Account))
	}
	if result.User.Comment != "" {
		lines = append(lines, fmt.Sprintf("comment: %s", result.User.Comment))
	}
	if webpage := publicWebpage(result.Profile.Webpage); webpage != "" {
		lines = append(lines, fmt.Sprintf("webpage: %s", webpage))
	}
	if result.Profile.Region != "" {
		lines = append(lines, fmt.Sprintf("region: %s", result.Profile.Region))
	}
	if result.Profile.CountryCode != "" {
		lines = append(lines, fmt.Sprintf("country: %s", result.Profile.CountryCode))
	}
	if result.Profile.Job != "" {
		lines = append(lines, fmt.Sprintf("job: %s", result.Profile.Job))
	}
	lines = append(lines,
		fmt.Sprintf("artworks: %d", result.Profile.TotalIllusts),
		fmt.Sprintf("manga: %d", result.Profile.TotalManga),
		fmt.Sprintf("novels: %d", result.Profile.TotalNovels),
		fmt.Sprintf("following: %d", result.Profile.TotalFollowUsers),
	)
	for _, field := range []struct {
		name  string
		value string
	}{
		{"workspace pc", result.Workspace.PC},
		{"workspace monitor", result.Workspace.Monitor},
		{"workspace tool", result.Workspace.Tool},
		{"workspace scanner", result.Workspace.Scanner},
		{"workspace tablet", result.Workspace.Tablet},
		{"workspace mouse", result.Workspace.Mouse},
		{"workspace printer", result.Workspace.Printer},
		{"workspace desktop", result.Workspace.Desktop},
		{"workspace music", result.Workspace.Music},
		{"workspace desk", result.Workspace.Desk},
		{"workspace chair", result.Workspace.Chair},
		{"workspace comment", result.Workspace.Comment},
	} {
		if field.value != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", field.name, field.value))
		}
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

// publicWebpage 将可显示的个人主页限定为有主机的 HTTP(S) 地址，并移除可能含有
// 私密信息的 userinfo、query 和 fragment。机器接口仍由 --json 返回完整 DTO。
func publicWebpage(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}
