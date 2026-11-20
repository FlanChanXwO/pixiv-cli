// Package vectormodel 把文字和图片转换成归一化向量。
//
// [Install] 安装固定本地资产，[Open] 绑定本地模型或云端服务；
// [Model.Probe] 用受控内容核对多模态能力。
// 模型身份包含服务地址、接口格式和模型 ID，配置持久化由调用方负责。
package vectormodel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ErrTextTooLong 表示本地查询超出含 EOS 的固定上下文长度。
var ErrTextTooLong = errors.New("local text query is too long")

const defaultBase = "https://generativelanguage.googleapis.com"

// Options 指定模型、本地资产目录和云端服务；APIKey 仅用于请求头。
type Options struct {
	Name, BaseURL     string
	Directory, Device string
	APIKey            string `json:"-"`
}

// Format 隐藏鉴权参数。
func (Options) Format(s fmt.State, _ rune) { fmt.Fprint(s, "vectormodel.Options{<private>}") }

// Model 持有独立的 HTTP client 配置；调用方保留 transport 的所有权。
type Model struct {
	options Options
	http    http.Client
	local   *localModel
}

// Format 避免诊断展开模型中的鉴权信息。
func (Model) Format(s fmt.State, _ rune) { fmt.Fprint(s, "vectormodel.Model{}") }

// CanonicalBase 规范化服务根地址，包括显式传入的终端 /v1。
func CanonicalBase(raw string) (string, error) {
	if raw == "" {
		raw = defaultBase
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("invalid vector service URL")
	}
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return "", errors.New("vector service URL requires HTTPS except on loopback")
	}
	u.RawPath = strings.TrimSuffix(strings.TrimRight(u.EscapedPath(), "/"), "/v1")
	u.Path, err = url.PathUnescape(u.RawPath)
	if err != nil {
		return "", errors.New("invalid vector service URL path")
	}
	if u.Port() == "443" && u.Scheme == "https" || u.Port() == "80" && u.Scheme == "http" {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	return u.String(), nil
}

var modelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Identifier 返回与鉴权和设备无关的向量身份。
func Identifier(name, base string) (string, error) {
	if !modelName.MatchString(name) {
		return "", errors.New("invalid vector model ID")
	}
	if spec, ok := localModels[name]; ok {
		return name + "@" + spec.revision, nil
	}
	canonical, err := CanonicalBase(base)
	if err != nil {
		return "", err
	}
	return "gemini-embedContent|" + canonical + "|" + name, nil
}

// Open 验证选项并阻止重定向把自定义鉴权头发送到另一服务。
func Open(options Options, client *http.Client) (*Model, error) {
	if _, err := Identifier(options.Name, options.BaseURL); err != nil {
		return nil, err
	}
	if IsLocal(options.Name) {
		local, err := openLocal(options)
		if err != nil {
			return nil, err
		}
		return &Model{options: options, local: local}, nil
	}
	base, err := CanonicalBase(options.BaseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.APIKey) == "" {
		return nil, errors.New("vector service API key is required; run \"pixiv vector setup\"")
	}
	options.BaseURL = base
	m := &Model{options: options, http: *client}
	m.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return m, nil
}

// Close 释放模型资源；HTTP transport 由调用方关闭。
func (m *Model) Close() error {
	if m.local != nil {
		return m.local.close()
	}
	return nil
}

// HTTPError 只公开状态码，服务响应正文和请求信息不进入错误。
type HTTPError struct{ Status int }

// Error 返回固定的 HTTP 状态摘要。
func (e *HTTPError) Error() string { return fmt.Sprintf("vector service returned HTTP %d", e.Status) }

// StatusCode 供同步器区分单页拒绝、认证和限流。
func (e *HTTPError) StatusCode() int { return e.Status }

// Text 把完整文字作为一次请求发送。
func (m *Model) Text(ctx context.Context, text string) ([]float32, error) {
	if m.local != nil {
		return m.local.textVector(ctx, text)
	}
	return m.embed(ctx, map[string]any{"text": text})
}

// Image 原样上传图片；尺寸限制由服务返回的状态码表达。
func (m *Model) Image(ctx context.Context, data []byte) ([]float32, error) {
	if m.local != nil {
		return m.local.image(ctx, data)
	}
	return m.embed(ctx, map[string]any{"inline_data": map[string]string{"mime_type": http.DetectContentType(data), "data": base64.StdEncoding.EncodeToString(data)}})
}

func (m *Model) embed(ctx context.Context, part any) (result []float32, err error) {
	body, err := json.Marshal(map[string]any{"model": "models/" + m.options.Name, "content": map[string]any{"parts": []any{part}}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.options.BaseURL+"/v1beta/models/"+m.options.Name+":embedContent", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("invalid vector request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", m.options.APIKey)
	response, err := m.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("vector service transport failed")
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			result = nil
			err = errors.Join(err, errors.New("vector response close failed"))
		}
	}()
	if response.StatusCode != http.StatusOK {
		return nil, &HTTPError{Status: response.StatusCode}
	}
	var value struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&value); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("invalid vector service response")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("invalid vector service response")
	}
	return normalize(value.Embedding.Values)
}

func normalize(values []float32) ([]float32, error) {
	var norm float64
	for _, v := range values {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, errors.New("vector contains a non-finite value")
		}
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		return nil, errors.New("vector is empty or zero")
	}
	norm = math.Sqrt(norm)
	for i := range values {
		values[i] = float32(float64(values[i]) / norm)
	}
	return values, nil
}

// ProbeImage 返回供 setup 和单页拒绝诊断使用的固定 PNG。
func ProbeImage() ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 230, G: 30, B: 40, A: 255})
		}
	}
	var out bytes.Buffer
	err := png.Encode(&out, img)
	return out.Bytes(), err
}

// Probe 要求同一模型的文字和图片向量具有相同维度。
func (m *Model) Probe(ctx context.Context) (int, error) {
	text, err := m.Text(ctx, "a red square")
	if err != nil {
		return 0, err
	}
	data, err := ProbeImage()
	if err != nil {
		return 0, err
	}
	img, err := m.Image(ctx, data)
	if err != nil {
		return 0, fmt.Errorf("multimodal image probe failed: %w", err)
	}
	if len(text) != len(img) {
		return 0, errors.New("text and image vector dimensions differ")
	}
	return len(text), nil
}
