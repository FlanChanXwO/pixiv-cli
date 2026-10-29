package user

import (
	"fmt"
	"io"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/plaintext"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// printUserSearchPreviews 展示搜索结果里的账号与简介，逐行转义上游文本，避免控制
// 字节破坏终端的逐行协议。
func printUserSearchPreviews(out io.Writer, items []pixiv.UserPreview) error {
	for _, item := range items {
		line := fmt.Sprintf("%d %s", item.User.ID, plaintext.SafeLine(item.User.Name))
		if item.User.Account != "" {
			line += " (@" + plaintext.SafeLine(item.User.Account) + ")"
		}
		if item.User.Comment != "" {
			line += " — " + plaintext.SafeLine(item.User.Comment)
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
		lines = append(lines, fmt.Sprintf("name: %s", plaintext.SafeLine(result.User.Name)))
	}
	if result.User.Account != "" {
		lines = append(lines, fmt.Sprintf("account: %s", plaintext.SafeLine(result.User.Account)))
	}
	if result.User.Comment != "" {
		lines = append(lines, fmt.Sprintf("comment: %s", plaintext.SafeLine(result.User.Comment)))
	}
	if webpage := publicWebpage(result.Profile.Webpage); webpage != "" {
		lines = append(lines, fmt.Sprintf("webpage: %s", plaintext.SafeLine(webpage)))
	}
	if result.Profile.Region != "" {
		lines = append(lines, fmt.Sprintf("region: %s", plaintext.SafeLine(result.Profile.Region)))
	}
	if result.Profile.CountryCode != "" {
		lines = append(lines, fmt.Sprintf("country: %s", plaintext.SafeLine(result.Profile.CountryCode)))
	}
	if result.Profile.Job != "" {
		lines = append(lines, fmt.Sprintf("job: %s", plaintext.SafeLine(result.Profile.Job)))
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
			lines = append(lines, fmt.Sprintf("%s: %s", field.name, plaintext.SafeLine(field.value)))
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
