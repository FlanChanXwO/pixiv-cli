// Package saucenao 实现 SauceNAO multipart JSON API 协议适配器。
package saucenao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
)

const defaultEndpoint = "https://saucenao.com/search.php"

// Options 是 SauceNAO adapter 的构造依赖。凭据不会进入每次查询请求模型。
type Options struct {
	APIKey     string
	HTTPClient *http.Client
	Endpoint   string
}

type Client struct {
	apiKey     string
	httpClient *http.Client
	endpoint   string
}

func New(options Options) *Client {
	endpoint := options.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{apiKey: options.APIKey, httpClient: client, endpoint: endpoint}
}

// Preflight 在 source 被读取前验证构造期凭据。
func (c *Client) Preflight(ctx context.Context) error {
	if ctx == nil {
		return reversesearch.NewError(reversesearch.CodeInvalidRequest, "reverse search context is required", nil)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil || strings.TrimSpace(c.apiKey) == "" {
		return reversesearch.NewError(reversesearch.CodeMissingCredential, "SauceNAO API key is required", nil)
	}
	return nil
}

// Search 上传同一私有快照并解析 SauceNAO JSON 响应。
func (c *Client) Search(ctx context.Context, snapshot *reversesearch.Snapshot) (reversesearch.ProviderResponse, error) {
	if err := c.Preflight(ctx); err != nil {
		return reversesearch.ProviderResponse{}, err
	}
	if snapshot == nil {
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeInvalidRequest, "image snapshot is required", nil)
	}
	body, contentType, writeResult := c.multipartBody(ctx, snapshot)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, body)
	if err != nil {
		_ = body.CloseWithError(err)
		<-writeResult
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not create SauceNAO request", nil)
	}
	request.Header.Set("Content-Type", contentType)
	response, err := c.httpClient.Do(request)
	if err != nil {
		_ = body.CloseWithError(err)
		<-writeResult
		if ctxErr := ctx.Err(); ctxErr != nil {
			return reversesearch.ProviderResponse{}, ctxErr
		}
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "SauceNAO request failed", nil)
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
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "could not upload image to SauceNAO", nil)
	}
	return decodeResponse(response.Body)
}

func (c *Client) multipartBody(ctx context.Context, snapshot *reversesearch.Snapshot) (*io.PipeReader, string, <-chan error) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	result := make(chan error, 1)
	go func() {
		err := writeMultipart(ctx, multipartWriter, snapshot, c.apiKey)
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
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

func writeMultipart(ctx context.Context, writer *multipart.Writer, snapshot *reversesearch.Snapshot, apiKey string) error {
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
	source, err := snapshot.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	_, err = io.Copy(part, contextReader{ctx: ctx, reader: source})
	return err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
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
		return reversesearch.ProviderResponse{}, malformedResponseError()
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return reversesearch.ProviderResponse{}, malformedResponseError()
	}
	if err := validateWireResponse(wire); err != nil {
		return reversesearch.ProviderResponse{}, malformedResponseError()
	}
	if *wire.Header.Status != 0 {
		return reversesearch.ProviderResponse{}, reversesearch.NewError(reversesearch.CodeProviderFailed, "SauceNAO rejected the query", nil)
	}
	response := reversesearch.ProviderResponse{
		Provider: reversesearch.ProviderSauceNAO,
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
			Title: result.Data.Title, Author: firstNonEmpty(result.Data.MemberName, result.Data.AuthorName),
			ArtworkID: int64(result.Data.PixivID), UserID: int64(result.Data.MemberID),
			ExternalURLs: append([]string(nil), result.Data.ExternalURLs...),
		})
	}
	return response, nil
}

func malformedResponseError() error {
	// JSON 解码错误可能包含上游字段原值，因此不把 cause 暴露到错误链。
	return reversesearch.NewError(reversesearch.CodeMalformedUpstreamResponse, "SauceNAO returned a malformed response", nil)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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

func ensureJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

var _ reversesearch.ProviderClient = (*Client)(nil)
