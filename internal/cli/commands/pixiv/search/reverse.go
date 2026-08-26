package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	textutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	"github.com/spf13/cobra"
)

// ReverseSearchRequest 是 search owner 传给反向搜图构造端口的单次请求。
// 具体 provider client、凭据和快照生命周期仍由上层组合与 Facade 管理。
type ReverseSearchRequest struct {
	Source             string
	Provider           reversesearch.Provider
	HTTPSProxyOverride *string
}

// ReverseSearchFunc 是 CLI owner 使用的反向搜图窄执行端口。
type ReverseSearchFunc func(context.Context, ReverseSearchRequest) (reversesearch.Response, error)

func (a command) runReverseSearch(cmd *cobra.Command, source string, opts options) error {
	if err := validateReverseSearchFlags(cmd); err != nil {
		return a.data.usage(err)
	}
	request, err := a.data.request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	provider, err := resolveReverseSearchProvider(opts.provider)
	if err != nil {
		return a.data.usage(err)
	}
	jsonOut := false
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.data.usage(errors.New("--ndjson cannot be used with --json"))
	}
	if a.data.ReverseSearch == nil {
		return errors.New("pixiv reverse search is not configured")
	}
	if !opts.ndjson {
		jsonOut, err = a.data.jsonOut(a.data.jsonOverride(cmd, opts.CommandOptions))
		if err != nil {
			return err
		}
	}
	ndjson := a.data.shouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	response, searchErr := a.data.ReverseSearch(cmd.Context(), ReverseSearchRequest{
		Source:             source,
		Provider:           provider,
		HTTPSProxyOverride: request.HTTPSProxyOverride,
	})
	if !reverseSearchResponseHasData(response) && searchErr != nil {
		return searchErr
	}
	if outputErr := a.writeReverseSearchResponse(response, jsonOut, ndjson); outputErr != nil {
		return outputErr
	}
	if response.Partial {
		if warningErr := a.writeReverseSearchWarning(response); warningErr != nil {
			return warningErr
		}
	}
	return searchErr
}

func resolveReverseSearchProvider(value string) (reversesearch.Provider, error) {
	if value == "" {
		return "", nil
	}
	provider := reversesearch.Provider(value)
	switch provider {
	case reversesearch.ProviderSauceNAO, reversesearch.ProviderASCII2DColor, reversesearch.ProviderASCII2DBOVW, reversesearch.ProviderAll:
		return provider, nil
	default:
		return "", errors.New("provider must be one of saucenao, ascii2d-color, ascii2d-bovw, all")
	}
}

func isReverseSearchSource(source string) bool {
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
		return true
	}
	info, err := os.Stat(source)
	return err == nil && info.Mode().IsRegular()
}

func validateReverseSearchFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("json") && cmd.Flags().Changed("ndjson") {
		return errors.New("--ndjson cannot be used with --json")
	}
	for _, name := range []string{
		"search-by", "sort", "period", "start-date", "end-date", "rating", "type", "content-type",
		"resolution", "aspect-ratio", "draw-tool", "ai-mode", "bookmark-min", "bookmark-max",
		"bookmark-strategy", "limit", "page",
	} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s is not supported for image sources", name)
		}
	}
	return nil
}

type reverseSearchOutput struct {
	Input          reversesearch.Input             `json:"input"`
	Providers      []reversesearch.ProviderSummary `json:"providers"`
	Results        []reversesearch.Result          `json:"results"`
	Records        []record.Record                 `json:"records"`
	ProviderErrors []reversesearch.ProviderError   `json:"provider_errors"`
	Partial        bool                            `json:"partial"`
}

func reverseSearchResponseHasData(response reversesearch.Response) bool {
	return response.Input.Kind != "" || response.Input.SHA256 != "" || len(response.Providers) > 0 ||
		len(response.Results) > 0 || len(response.ProviderErrors) > 0 || response.Partial
}

func (a command) writeReverseSearchResponse(response reversesearch.Response, jsonOut, ndjson bool) error {
	records, err := reverseSearchRecords(response.Results)
	if err != nil {
		return err
	}
	if ndjson {
		encoder := json.NewEncoder(a.data.Output)
		for _, item := range records {
			if err := encoder.Encode(item); err != nil {
				return err
			}
		}
		return nil
	}
	if jsonOut {
		return a.data.writeJSON(reverseSearchOutput{
			Input: response.Input, Providers: nonNilProviderSummaries(response.Providers), Results: nonNilReverseResults(response.Results),
			Records: records, ProviderErrors: nonNilProviderErrors(response.ProviderErrors), Partial: response.Partial,
		})
	}
	return printReverseSearchHuman(a.data.Output, response.Results)
}

func reverseSearchRecords(results []reversesearch.Result) ([]record.Record, error) {
	records := make([]record.Record, 0, len(results))
	for _, result := range results {
		if result.Pixiv == nil {
			continue
		}
		typeName, path, identityErr := reverseSearchIdentity(result.Pixiv)
		if identityErr != nil {
			return nil, identityErr
		}
		identity, err := record.NewIdentityRecord(result.Pixiv.ID, typeName, path)
		if err != nil {
			return nil, errors.New("reverse search returned an invalid Pixiv identity")
		}
		records = append(records, identity)
	}
	return records, nil
}

func reverseSearchIdentity(ref *reversesearch.PixivRef) (recordType, rawURL string, err error) {
	if ref == nil {
		return "", "", errors.New("reverse search returned an invalid Pixiv identity")
	}
	switch ref.Type {
	case reversesearch.PixivRefUser:
		return "user", fmt.Sprintf("https://www.pixiv.net/users/%d", ref.ID), nil
	case reversesearch.PixivRefArtwork:
		return "artwork", fmt.Sprintf("https://www.pixiv.net/artworks/%d", ref.ID), nil
	default:
		return "", "", errors.New("reverse search returned an invalid Pixiv identity")
	}
}

func printReverseSearchHuman(out io.Writer, results []reversesearch.Result) error {
	for _, result := range results {
		line := "external result"
		if result.Pixiv != nil {
			typeName, rawURL, identityErr := reverseSearchIdentity(result.Pixiv)
			if identityErr != nil {
				return identityErr
			}
			identity, err := record.NewIdentityRecord(result.Pixiv.ID, typeName, rawURL)
			if err != nil {
				return errors.New("reverse search returned an invalid Pixiv identity")
			}
			line = identity.URL()
		}
		if title := textutil.SafeLine(result.Title); title != "" {
			line += " — " + title
		}
		if author := textutil.SafeLine(result.Author); author != "" {
			line += " by " + author
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func (a command) writeReverseSearchWarning(response reversesearch.Response) error {
	if a.data.ErrorOutput == nil {
		return errors.New("reverse search partial warning output is not configured")
	}
	failed := make([]string, 0, len(response.ProviderErrors))
	for _, providerError := range response.ProviderErrors {
		failed = append(failed, string(providerError.Provider))
	}
	if len(failed) == 0 {
		failed = append(failed, "unknown provider")
	}
	_, err := fmt.Fprintf(a.data.ErrorOutput, "warning: reverse search completed partially; failed providers: %s\n", strings.Join(failed, ", "))
	return err
}

func nonNilProviderSummaries(values []reversesearch.ProviderSummary) []reversesearch.ProviderSummary {
	if values == nil {
		return []reversesearch.ProviderSummary{}
	}
	return values
}

func nonNilReverseResults(values []reversesearch.Result) []reversesearch.Result {
	if values == nil {
		return []reversesearch.Result{}
	}
	return values
}

func nonNilProviderErrors(values []reversesearch.ProviderError) []reversesearch.ProviderError {
	if values == nil {
		return []reversesearch.ProviderError{}
	}
	return values
}
