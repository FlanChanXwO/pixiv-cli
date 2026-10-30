package display

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PrintUserPreviews 转义用户名，每个用户预览输出一行。
func PrintUserPreviews(out io.Writer, users []pixiv.UserPreview) error {
	for _, item := range users {
		if _, err := fmt.Fprintln(out, Identity(item.User.Name, item.User.ID)); err != nil {
			return err
		}
	}
	return nil
}

// PrintUserSearchPreviews 展示搜索结果里的账号与简介，逐行转义上游文本，避免控制
// 字节破坏终端的逐行协议。
func PrintUserSearchPreviews(out io.Writer, items []pixiv.UserPreview) error {
	return PrintUserPreviews(out, items)
}

// PrintUserDetail 只展示人可读且有值的文本字段；固定计数保留零值，以免把公开的
// “没有作品/关注”误报为字段缺失。完整机器可读字段由 --json 原样输出 SDK DTO。
func PrintUserDetail(out io.Writer, result pixiv.UserDetail) error {
	identity := Identity(result.User.Name, result.User.ID)
	if result.User.Account != "" {
		identity = fmt.Sprintf("%s (@%s, %d)", SafeLine(result.User.Name), SafeLine(result.User.Account), result.User.ID)
	}
	lines := []string{identity}
	if result.User.Comment != "" {
		lines = append(lines, SafeLine(result.User.Comment))
	}
	lines = append(lines, fmt.Sprintf("%d artworks · %d manga · %d novels · %d following", result.Profile.TotalIllusts, result.Profile.TotalManga, result.Profile.TotalNovels, result.Profile.TotalFollowUsers))
	var profile []string
	if webpage := publicWebpage(result.Profile.Webpage); webpage != "" {
		profile = append(profile, SafeLine(webpage))
	}
	for _, value := range []string{result.Profile.Region, result.Profile.CountryCode, result.Profile.Job} {
		if value != "" {
			profile = append(profile, SafeLine(value))
		}
	}
	if len(profile) > 0 {
		lines = append(lines, strings.Join(profile, " · "))
	}

	for _, field := range []struct {
		name  string
		value string
	}{
		{"pc", result.Workspace.PC},
		{"monitor", result.Workspace.Monitor},
		{"tool", result.Workspace.Tool},
		{"scanner", result.Workspace.Scanner},
		{"tablet", result.Workspace.Tablet},
		{"mouse", result.Workspace.Mouse},
		{"printer", result.Workspace.Printer},
		{"desktop", result.Workspace.Desktop},
		{"music", result.Workspace.Music},
		{"desk", result.Workspace.Desk},
		{"chair", result.Workspace.Chair},
		{"comment", result.Workspace.Comment},
	} {
		if field.value != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", field.name, SafeLine(field.value)))
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
