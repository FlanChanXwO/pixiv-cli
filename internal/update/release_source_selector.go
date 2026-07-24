package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type releaseSourceKind string

const (
	releaseSourceAPI   releaseSourceKind = "GitHub Releases API"
	releaseSourceAsset releaseSourceKind = "release asset"
)

// releaseSourceSelector 以同一个调用 context 并发探测内置传输路径；首个返回可用响应的
// 候选即为本次首选。显式 update 不添加人为截止时间，自动检查沿用调用方已有的总截止时间。
type releaseSourceSelector struct {
	sources    []releaseSource
	httpClient *http.Client
}

func newReleaseSourceSelector(sources []releaseSource, httpClient *http.Client) *releaseSourceSelector {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &releaseSourceSelector{sources: append([]releaseSource(nil), sources...), httpClient: httpClient}
}

func newDefaultReleaseSourceSelector(httpClient *http.Client) *releaseSourceSelector {
	return newReleaseSourceSelector(defaultReleaseSources(), httpClient)
}

// ordered 返回首个通过探测的候选，随后按内置顺序附上尚未使用的同类候选，供下载失败时逐一
// 尝试。候选只改写 canonical URL；后续签名、checksum 与 URL allowlist 仍在调用处执行。
func (s *releaseSourceSelector) ordered(ctx context.Context, kind releaseSourceKind, canonicalURL string) ([]releaseSource, error) {
	if s == nil {
		return nil, fmt.Errorf("release source selector is nil")
	}
	if err := checkContext(ctx, "select release source"); err != nil {
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
	source releaseSource
	err    error
}

func (s *releaseSourceSelector) candidates(kind releaseSourceKind) []releaseSource {
	var candidates []releaseSource
	for _, source := range s.sources {
		supported := source.asset.raw != ""
		if kind == releaseSourceAPI {
			supported = source.api.raw != ""
		}
		if supported {
			candidates = append(candidates, source)
		}
	}
	return candidates
}

func (s *releaseSourceSelector) probe(ctx context.Context, source releaseSource, kind releaseSourceKind, canonicalURL string) error {
	transformedURL, err := source.assetURL(canonicalURL)
	if kind == releaseSourceAPI {
		transformedURL, err = source.apiURL(canonicalURL)
	}
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, transformedURL, nil)
	if err != nil {
		return fmt.Errorf("create probe request: %w", err)
	}
	request.Header.Set("User-Agent", githubReleaseUserAgent)
	response, err := s.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request probe URL: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("probe URL returned HTTP %s", response.Status)
	}
	if kind != releaseSourceAPI {
		_, err := io.Copy(io.Discard, response.Body)
		if err != nil {
			return fmt.Errorf("read probe response: %w", err)
		}
		return nil
	}
	var releases []githubRelease
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

func placeReleaseSourceFirst(first releaseSource, sources []releaseSource) []releaseSource {
	ordered := make([]releaseSource, 0, len(sources))
	ordered = append(ordered, first)
	for _, source := range sources {
		if source.id != first.id {
			ordered = append(ordered, source)
		}
	}
	return ordered
}

func firstReleaseSource(sources []releaseSource) *releaseSource {
	if len(sources) == 0 {
		return nil
	}
	return &sources[0]
}
