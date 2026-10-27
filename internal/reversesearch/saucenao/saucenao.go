// Package saucenao 实现 SauceNAO multipart JSON API 协议适配器。
//
// 包的组成：[New] 按 [Options] 构造 [Client]，[Client] 实现 reversesearch 的
// provider 端口，负责上传图片并映射结果与额度。
package saucenao

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
)

const defaultEndpoint = "https://saucenao.com/search.php"

// Options 提供 SauceNAO adapter 的构造依赖。凭据不会进入每次查询请求模型。
type Options struct {
	APIKey     string
	HTTPClient *http.Client
}

// Client 通过 SauceNAO multipart API 上传图片并返回匹配结果与额度。
type Client struct {
	apiKey     string
	httpClient *http.Client
}

var _ reversesearch.Closer = (*Client)(nil)

// New 使用 options 中的 API key 与 HTTP client 构造 SauceNAO client。
func New(options Options) *Client {
	return &Client{apiKey: options.APIKey, httpClient: options.HTTPClient}
}

// Close 释放 SauceNAO client 的空闲连接；整个 reverse-search Service 结束时
// 调用一次即可，重复调用保持幂等。
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// Preflight 在 source 被读取前验证构造期凭据。
func (c *Client) Preflight(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return reversesearch.NewError(reversesearch.CodeMissingCredential, "SauceNAO API key is required", nil)
	}
	return nil
}

// Search 上传同一私有快照并解析 SauceNAO JSON 响应。
func (c *Client) Search(ctx context.Context, snapshot *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
	if err := ctx.Err(); err != nil {
		return reversesearch.ProviderResponse{}, err
	}
	body, contentType, writeResult := c.multipartBody(ctx, snapshot)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, defaultEndpoint, body)
	if err != nil {
		_ = body.CloseWithError(err)
		<-writeResult
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create SauceNAO request", err)
	}
	request.Header.Set("Content-Type", contentType)
	response, err := c.httpClient.Do(request)
	if err != nil {
		_ = body.CloseWithError(err)
		<-writeResult
		if ctxErr := ctx.Err(); ctxErr != nil {
			return reversesearch.ProviderResponse{}, ctxErr
		}
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "SauceNAO request failed", err)
	}
	defer response.Body.Close()
	writeErr := <-writeResult
	if ctxErr := ctx.Err(); ctxErr != nil {
		return reversesearch.ProviderResponse{}, ctxErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeUpstreamHTTPStatus, "SauceNAO returned an unsuccessful HTTP status", nil)
	}
	if writeErr != nil {
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not upload image to SauceNAO", writeErr)
	}
	return decodeResponse(response.Body)
}

func (c *Client) multipartBody(ctx context.Context, snapshot *reversesearch.Snapshot) (*io.PipeReader, string, <-chan error) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	result := make(chan error, 1)
	go func() {
		err := writeMultipart(ctx, multipartWriter, snapshot, c.apiKey)
		err = errors.Join(err, multipartWriter.Close())
		if err != nil {
			_ = writer.CloseWithError(err)
		} else {
			err = writer.Close()
		}
		result <- err
		close(result)
	}()
	return reader, multipartWriter.FormDataContentType(), result
}

func writeMultipart(ctx context.Context, writer *multipart.Writer, snapshot *reversesearch.Snapshot, apiKey string) (err error) {
	fields := []struct{ name, value string }{
		{name: "api_key", value: apiKey},
		{name: "output_type", value: "2"},
		{name: "db", value: "999"},
	}
	for _, field := range fields {
		if err := writer.WriteField(field.name, field.value); err != nil {
			return err
		}
	}
	part, err := writer.CreateFormFile("file", "image")
	if err != nil {
		return err
	}
	source, err := snapshot.Open(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, source.Close()) }()
	_, err = io.Copy(part, source)
	return err
}

type flexibleInt int64

func (value *flexibleInt) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(string(raw), `"`)
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return err
	}
	*value = flexibleInt(parsed)
	return nil
}

type flexibleFloat float64

func (value *flexibleFloat) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(string(raw), `"`)
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return err
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return errors.New("floating-point value must be finite")
	}
	*value = flexibleFloat(parsed)
	return nil
}

type wireResponse struct {
	Header  *wireResponseHeader `json:"header"`
	Results *[]wireResult       `json:"results"`
}

type wireResponseHeader struct {
	Status         *flexibleInt `json:"status"`
	ShortRemaining *flexibleInt `json:"short_remaining"`
	LongRemaining  *flexibleInt `json:"long_remaining"`
	ShortLimit     *flexibleInt `json:"short_limit"`
	LongLimit      *flexibleInt `json:"long_limit"`
}

type wireResult struct {
	Header *wireResultHeader `json:"header"`
	Data   *wireResultData   `json:"data"`
}

type wireResultHeader struct {
	Similarity *flexibleFloat `json:"similarity"`
	IndexID    *flexibleInt   `json:"index_id"`
	IndexName  string         `json:"index_name"`
}

type wireResultData struct {
	ExternalURLs []string    `json:"ext_urls"`
	Title        string      `json:"title"`
	PixivID      flexibleInt `json:"pixiv_id"`
	MemberName   string      `json:"member_name"`
	AuthorName   string      `json:"author_name"`
	MemberID     flexibleInt `json:"member_id"`
}

func decodeResponse(body io.Reader) (reversesearch.ProviderResponse, error) {
	decoder := json.NewDecoder(body)
	var wire wireResponse
	if err := decoder.Decode(&wire); err != nil {
		return reversesearch.ProviderResponse{}, malformedResponseError(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return reversesearch.ProviderResponse{}, malformedResponseError(err)
	}
	if err := validateWireResponse(wire); err != nil {
		return reversesearch.ProviderResponse{}, malformedResponseError(err)
	}
	if *wire.Header.Status != 0 {
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "SauceNAO rejected the query", nil)
	}
	response := reversesearch.ProviderResponse{
		Quota: &reversesearch.Quota{
			ShortRemaining: int(*wire.Header.ShortRemaining), LongRemaining: int(*wire.Header.LongRemaining),
			ShortLimit: int(*wire.Header.ShortLimit), LongLimit: int(*wire.Header.LongLimit),
		},
		Matches: make([]reversesearch.Match, 0, len(*wire.Results)),
	}
	for index, result := range *wire.Results {
		response.Matches = append(response.Matches, reversesearch.Match{
			Rank: index + 1, Similarity: float64(*result.Header.Similarity),
			IndexID: int(*result.Header.IndexID), IndexName: result.Header.IndexName,
			Title: result.Data.Title, Author: cmp.Or(result.Data.MemberName, result.Data.AuthorName),
			ArtworkID: int64(result.Data.PixivID), UserID: int64(result.Data.MemberID),
			ExternalURLs: result.Data.ExternalURLs,
		})
	}
	return response, nil
}

func malformedResponseError(cause error) error {
	return reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "SauceNAO returned a malformed response", cause)
}

func validateWireResponse(response wireResponse) error {
	if response.Header == nil || response.Header.Status == nil {
		return errors.New("missing response header or status")
	}
	if *response.Header.Status != 0 {
		return nil
	}
	if response.Results == nil {
		return errors.New("missing response results")
	}
	if response.Header.ShortRemaining == nil || response.Header.LongRemaining == nil || response.Header.ShortLimit == nil || response.Header.LongLimit == nil {
		return errors.New("missing quota fields")
	}
	for _, result := range *response.Results {
		if result.Header == nil || result.Data == nil || result.Header.Similarity == nil || result.Header.IndexID == nil || result.Header.IndexName == "" {
			return errors.New("missing result fields")
		}
	}
	return nil
}

var _ reversesearch.ProviderClient = (*Client)(nil)
