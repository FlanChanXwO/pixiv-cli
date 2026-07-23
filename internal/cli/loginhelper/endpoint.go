package loginhelper

import (
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const callbackEndpointFilename = "url-handler-endpoint"

// CallbackCommand 是 desktop protocol handler 调回当前 CLI binary 的隐藏入口名。
// 它只由本包生成的系统关联使用，不是公开 CLI 命令。
const CallbackCommand = "_callback"

// callbackEndpointPath 是系统协议 handler 与当前登录进程之间唯一共享的私有状态。
// 文件只保存 loopback bridge 地址，永远不保存 authorization code 或 refresh token。
func callbackEndpointPath() (string, error) {
	dir, err := files.UserDataSubdir(constants.AppDataDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, callbackEndpointFilename), nil
}

func writeCallbackEndpoint(callbackRelayURL string) (string, error) {
	endpoint, err := validatedCallbackEndpoint(callbackRelayURL)
	if err != nil {
		return "", err
	}
	path, err := callbackEndpointPath()
	if err != nil {
		return "", err
	}
	if err := files.WritePrivateFile(path, []byte(endpoint+"\n"), constants.PrivateFileMode); err != nil {
		return "", err
	}
	return path, nil
}

// CallbackRelayURL 将系统交给 handler 的 pixiv:// URL 放入 bridge fragment。
// fragment 不会进入 loopback GET 或浏览器历史；bridge 页面会在 POST ��清除它。
func CallbackRelayURL(callbackURL string) (string, error) {
	callback, err := url.Parse(strings.TrimSpace(callbackURL))
	if err != nil || !isPixivCallback(callback) {
		return "", errors.New("invalid Pixiv callback URL")
	}
	path, err := callbackEndpointPath()
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("Pixiv login callback is no longer active")
	}
	endpoint, err := validatedCallbackEndpoint(string(body))
	if err != nil {
		return "", errors.New("Pixiv login callback endpoint is invalid")
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return "", errors.New("Pixiv login callback endpoint is invalid")
	}
	parsedEndpoint.Fragment = callbackURL
	return parsedEndpoint.String(), nil
}

func validatedCallbackEndpoint(raw string) (string, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint == nil || !strings.EqualFold(endpoint.Scheme, "http") || endpoint.Host == "" || endpoint.Path != "/callback" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return "", errors.New("invalid callback endpoint")
	}
	host, _, err := net.SplitHostPort(endpoint.Host)
	if err != nil {
		return "", errors.New("invalid callback endpoint")
	}
	if strings.EqualFold(host, "localhost") {
		return endpoint.String(), nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("callback endpoint must be loopback")
	}
	return endpoint.String(), nil
}

func isPixivCallback(parsed *url.URL) bool {
	return parsed != nil && strings.EqualFold(parsed.Scheme, "pixiv") && strings.EqualFold(parsed.Host, "account") && parsed.Path == "/login" && strings.TrimSpace(parsed.Query().Get("code")) != ""
}
