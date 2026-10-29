package main_test

import (
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/auth"
)

// TestBrowserProvidersStayRegistered 覆盖 fanbox auth 的 blank import：删掉任一 provider
// 的子包导入，这里会先于真实浏览器访问失败。
func TestBrowserProvidersStayRegistered(t *testing.T) {
	for _, name := range []string{"chrome", "edge", "firefox", "safari"} {
		provider, err := browsercookies.New(name)
		if errors.Is(err, browsercookies.ErrUnknownBrowser) {
			t.Fatalf("browser provider %q is not registered", name)
		}
		if err == nil {
			_ = provider.Close()
		}
	}
}
