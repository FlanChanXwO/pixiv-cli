package pixiv_test

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// Example shows the beginner SDK surface. Examples without Output are compiled
// by go test but do not contact Pixiv or require a local account.
func Example() {
	ctx := context.Background()
	client, err := pixiv.OpenDefault()
	if err != nil {
		return
	}

	// PID、作品页 URL 与受 ResourcePolicy 允许的 CDN URL 都能作为 src。
	_, _ = client.Download(ctx, "123456")
	_, _ = client.Download(ctx, "https://www.pixiv.net/artworks/123456")
	_, _ = client.Download(ctx, "https://i.pximg.net/img-original/example.jpg")

	_, _ = client.SearchIllust(ctx, pixiv.SearchIllustRequest{Word: "初音ミク"})
	_, _ = client.IllustDetail(ctx, 123456)
	// NextCursor 是不透明值，原样放回同一请求的 Cursor 字段即可继续分页。
	_, _ = client.SearchIllust(ctx, pixiv.SearchIllustRequest{Word: "初音ミク", Cursor: pixiv.Cursor("next")})
	_, _ = client.GetConfig(pixiv.ConfigKeyDownloadPath)
}

// ExampleClient_DownloadWith shows explicit output and concurrency control.
func ExampleClient_DownloadWith() {
	ctx := context.Background()
	client, err := pixiv.OpenDefaultWith(pixiv.OpenDefaultOptions{})
	if err != nil {
		return
	}
	_, _ = client.DownloadAllWith(ctx, []string{"123456", "123457"}, pixiv.DownloadOptions{
		DownloadPath: "./artwork-output",
		Pages:        []int{1, 3},
		Quality:      pixiv.DownloadQualityRegular,
		Concurrency:  8,
	})
}
