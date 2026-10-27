// Package system 注册全部浏览器 cookie provider。
//
// 导入本包会运行 chromium、firefox、safari 的 init 注册，之后 [New] 可以创建任意
// 受支持的 provider；browsercookies 根包本身保持与 provider 无关。
package system

import (
	// 导入 provider 会执行各自的 init 注册。
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/chromium"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/firefox"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/safari"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

// New 按 browser 名称创建已注册的 provider。
func New(browser string) (browsercookies.Provider, error) { return browsercookies.New(browser) }
