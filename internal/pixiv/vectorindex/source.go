package vectorindex

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
)

// ImageSource 识别 URL、显式图片路径和实际图片文件；普通同名文件保留文字含义。
func ImageSource(value string) (bool, error) {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
		return true, nil
	}
	explicit := filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "~/")
	switch strings.ToLower(filepath.Ext(value)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tiff", ".tif", ".avif":
		explicit = true
	}
	info, err := os.Stat(value)
	if err != nil {
		// 普通文字可能超出文件名长度；成功定位文件后才检查图片内容。
		if !explicit {
			return false, nil
		}
		return false, reversesearch.NewError(reversesearch.CodeSourceReadFailed, "could not read image source", err)
	}
	if !info.Mode().IsRegular() {
		if explicit {
			return false, reversesearch.NewError(reversesearch.CodeSourceNotRegularFile, "image source must be a regular file", nil)
		}
		return false, nil
	}
	file, err := os.Open(value)
	if err != nil {
		return false, reversesearch.NewError(reversesearch.CodeSourceReadFailed, "could not read image source", err)
	}
	_, _, err = image.Decode(file)
	if closeErr := file.Close(); closeErr != nil {
		return false, reversesearch.NewError(reversesearch.CodeSourceReadFailed, "could not close image source", errors.Join(err, closeErr))
	}
	if err != nil && explicit {
		return false, reversesearch.NewError(reversesearch.CodeInvalidSource, "image source has invalid or unsupported image content", err)
	}
	return err == nil, nil
}

// LoadImage 读取并验证实际图片内容；HTTP 与文件错误保留原始原因，诊断隐藏私有地址。
func LoadImage(ctx context.Context, client *http.Client, source string) (data []byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var input io.ReadCloser
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
		validate := func(raw string) error {
			parsed, err := url.Parse(raw)
			if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "http" && parsed.Scheme != "https" {
				return reversesearch.NewError(reversesearch.CodeInvalidSource, "image source must be an HTTP(S) URL without user information", err)
			}
			return nil
		}
		if err = validate(source); err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, reversesearch.NewError(reversesearch.CodeInvalidSource, "image source URL is invalid", err)
		}
		httpClient := *client
		httpClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
			if err := validate(next.URL.String()); err != nil {
				return err
			}
			if client.CheckRedirect != nil {
				return client.CheckRedirect(next, via)
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
		response, err := httpClient.Do(request)
		if err != nil {
			return nil, reversesearch.NewError(reversesearch.CodeSourceReadFailed, "could not fetch image source", err)
		}
		input = response.Body
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, errors.Join(reversesearch.NewError(reversesearch.CodeSourceHTTPStatus, "image source returned an unsuccessful HTTP status", nil), input.Close())
		}
	} else {
		info, err := os.Stat(source)
		if err != nil {
			return nil, reversesearch.NewError(reversesearch.CodeSourceReadFailed, "could not read image source", err)
		}
		if !info.Mode().IsRegular() {
			return nil, reversesearch.NewError(reversesearch.CodeSourceNotRegularFile, "image source must be a regular file", nil)
		}
		file, err := os.Open(source)
		if err != nil {
			return nil, reversesearch.NewError(reversesearch.CodeSourceReadFailed, "could not read image source", err)
		}
		input = file
		info, err = file.Stat()
		if err != nil {
			return nil, errors.Join(err, input.Close())
		}
		if !info.Mode().IsRegular() {
			return nil, errors.Join(reversesearch.NewError(reversesearch.CodeSourceNotRegularFile, "image source must be a regular file", nil), input.Close())
		}
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	data, err = io.ReadAll(input)
	if err != nil {
		return nil, reversesearch.NewError(reversesearch.CodeSourceReadFailed, "could not read image source", err)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return nil, reversesearch.NewError(reversesearch.CodeInvalidSource, "image source has invalid or unsupported image content", err)
	}
	return data, nil
}
