package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ReleaseSourceKind 描述候选传输路径服务的请求类别。
type ReleaseSourceKind string

const (
	ReleaseSourceAPI   ReleaseSourceKind = "GitHub Releases API"
	ReleaseSourceAsset ReleaseSourceKind = "release asset"
)

// releaseProbeWire 是探测 GitHub Releases API 路由时可识别的最小 wire 形状；
// 探测只校验响应是合法数组，不读取业务字段。
type releaseProbeWire struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// ReleaseSourceSelector 以同一个调用 context 并发探测内置传输路径；首个返回可用响应的
// 候选即为本次首选。显式 update 不添加人为截止时间，自动检查沿用调用方已有的总截止时间。
type ReleaseSourceSelector struct {
	sources    []ReleaseSource
	httpClient *http.Client
}

// NewReleaseSourceSelector 建立使用给定候选的传输路径选择器。
func NewReleaseSourceSelector(sources []ReleaseSource, httpClient *http.Client) *ReleaseSourceSelector {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &ReleaseSourceSelector{sources: append([]ReleaseSource(nil), sources...), httpClient: httpClient}
}

// NewDefaultReleaseSourceSelector 建立使用内置默认传输路径的选择器。
func NewDefaultReleaseSourceSelector(httpClient *http.Client) *ReleaseSourceSelector {
	return NewReleaseSourceSelector(DefaultReleaseSources(), httpClient)
}

// Ordered 返回首个通过探测的候选，随后按内置顺序附上尚未使用的同类候选，供下载失败时逐一
// 尝试。候选只改写 canonical URL；后续签名、checksum 与 URL allowlist 仍在调用处执行。
func (s *ReleaseSourceSelector) Ordered(ctx context.Context, kind ReleaseSourceKind, canonicalURL string) ([]ReleaseSource, error) {
	if s == nil {
		return nil, fmt.Errorf("release source selector is nil")
	}
	if err := CheckContext(ctx, "select release source"); err != nil {
		return nil, err
	}
	candidates := s.candidates(kind)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no release source supports %s", kind)
	}

	probeContext, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan releaseSourceProbeResult, len(candidates))
	for _, source := range candidates {
		source := source
		go func() {
			err := s.probe(probeContext, source, kind, canonicalURL)
			results <- releaseSourceProbeResult{source: source, err: err}
		}()
	}

	errorsBySource := make([]error, 0, len(candidates))
	for range candidates {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("select release source: %w", ctx.Err())
		case result := <-results:
			if result.err == nil {
				return placeReleaseSourceFirst(result.source, candidates), nil
			}
			errorsBySource = append(errorsBySource, fmt.Errorf("release source %q: %w", result.source.id, result.err))
		}
	}
	return nil, fmt.Errorf("no release source returned a valid %s response: %w", kind, errors.Join(errorsBySource...))
}

type releaseSourceProbeResult struct {
	source ReleaseSource
	err    error
}

func (s *ReleaseSourceSelector) candidates(kind ReleaseSourceKind) []ReleaseSource {
	var candidates []ReleaseSource
	for _, source := range s.sources {
		supported := source.asset.raw != ""
		if kind == ReleaseSourceAPI {
			supported = source.api.raw != ""
		}
		if supported {
			candidates = append(candidates, source)
		}
	}
	return candidates
}

func (s *ReleaseSourceSelector) probe(ctx context.Context, source ReleaseSource, kind ReleaseSourceKind, canonicalURL string) error {
	transformedURL, err := source.AssetURL(canonicalURL)
	if kind == ReleaseSourceAPI {
		transformedURL, err = source.APIURL(canonicalURL)
	}
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, transformedURL, nil)
	if err != nil {
		return fmt.Errorf("create probe request: %w", err)
	}
	request.Header.Set("User-Agent", GitHubUserAgent)
	response, err := s.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request probe URL: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("probe URL returned HTTP %s", response.Status)
	}
	if kind != ReleaseSourceAPI {
		_, err := io.Copy(io.Discard, response.Body)
		if err != nil {
			return fmt.Errorf("read probe response: %w", err)
		}
		return nil
	}
	var releases []releaseProbeWire
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&releases); err != nil {
		return fmt.Errorf("decode GitHub Releases JSON: %w", err)
	}
	if releases == nil {
		return fmt.Errorf("decode GitHub Releases JSON: expected an array")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode GitHub Releases JSON: contains more than one JSON value")
		}
		return fmt.Errorf("decode GitHub Releases JSON trailing data: %w", err)
	}
	return nil
}

func placeReleaseSourceFirst(first ReleaseSource, sources []ReleaseSource) []ReleaseSource {
	ordered := make([]ReleaseSource, 0, len(sources))
	ordered = append(ordered, first)
	for _, source := range sources {
		if source.id != first.id {
			ordered = append(ordered, source)
		}
	}
	return ordered
}

// FirstReleaseSource 返回有序候选中的首个源；列表为空时返回 nil。
func FirstReleaseSource(sources []ReleaseSource) *ReleaseSource {
	if len(sources) == 0 {
		return nil
	}
	return &sources[0]
}
