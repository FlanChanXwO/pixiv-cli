package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type githubClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type githubHTTPError struct {
	Path       string
	StatusCode int
	Body       string
}

// Error 输出路径、HTTP 状态码和响应体，便于在 workflow 日志里定位失败的 API 调用。
func (err *githubHTTPError) Error() string {
	if err.Body == "" {
		return fmt.Sprintf("GitHub API %s: HTTP %d", err.Path, err.StatusCode)
	}
	return fmt.Sprintf("GitHub API %s: HTTP %d: %s", err.Path, err.StatusCode, err.Body)
}

func (client githubClient) getJSON(ctx context.Context, path string, destination any) error {
	return client.requestJSON(ctx, http.MethodGet, path, nil, destination)
}

func (client githubClient) requestJSON(ctx context.Context, method, path string, input, destination any) error {
	baseURL := strings.TrimRight(client.baseURL, "/")
	if baseURL == "" {
		return errors.New("GitHub API base URL is required")
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode GitHub API request: %w", err)
		}
		body = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || (method == http.MethodGet && response.StatusCode != http.StatusOK) {
		responseBody, readErr := io.ReadAll(response.Body)
		return errors.Join(&githubHTTPError{Path: path, StatusCode: response.StatusCode, Body: strings.TrimSpace(string(responseBody))}, readErr)
	}
	if destination == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(destination)
}

func githubRepositoryPath(repository string) (string, error) {
	owner, name, ok := strings.Cut(repository, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("repository must use owner/name form: %q", repository)
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name), nil
}
