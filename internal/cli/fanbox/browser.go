package fanbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

// SystemBrowserProvider 把浏览器 cookie provider 的受约束读取结果
// 转换为 FANBOX auth import 所需的单一 session。它不打印或格式化 Secret，
// 也不会把浏览器路径带入用户可见错误。
type SystemBrowserProvider struct{}

func (SystemBrowserProvider) ReadSession(ctx context.Context, browser, profileID string) (session string, returnErr error) {
	provider, err := browsercookies.New(browser)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := provider.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, closeErr)
		}
	}()
	profiles, err := provider.DiscoverProfiles(ctx)
	if err != nil {
		return "", err
	}
	profile, err := browsercookies.SelectProfile(profiles, profileID)
	if err != nil {
		return "", err
	}
	secrets, err := provider.Read(ctx, browsercookies.DefaultQuery, profile.ID)
	if err != nil {
		return "", err
	}
	if len(secrets) == 0 {
		return "", errors.New("browser profile does not contain a FANBOXSESSID cookie")
	}
	if len(secrets) > 1 {
		return "", fmt.Errorf("browser profile contains multiple FANBOXSESSID cookies")
	}
	return secrets[0].Value(), nil
}
