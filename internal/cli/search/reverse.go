package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/spf13/cobra"
)

// ReverseSearchRequest 保存 search owner 传给反向搜图端口的一次请求。
// 具体 provider client、凭据和快照生命周期仍由上层组合与 reversesearch.Service 管理。
type ReverseSearchRequest struct {
	Source             string
	Provider           reversesearch.Provider
	HTTPSProxyOverride *string
}

// ReverseSearchFunc 让 CLI owner 在不依赖 provider 包的情况下执行反向搜图。
type ReverseSearchFunc func(context.Context, ReverseSearchRequest) (reversesearch.Response, error)

func (a commandRunner) runReverseSearch(cmd *cobra.Command, source string, opts options) error {
	if err := validateReverseSearchFlags(cmd); err != nil {
		return a.env.WrapUsage(err)
	}
	request, err := a.env.Request(cmd, opts.Options.ProxyOptions)
	if err != nil {
		return err
	}
	provider, err := resolveReverseSearchProvider(opts.provider)
	if err != nil {
		return a.env.WrapUsage(err)
	}
	jsonOut := false
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.env.WrapUsage(i18n.Errorf("error.ndjson_cannot_be_used_with_json"))
	}
	if !opts.ndjson {
		jsonOut, err = a.env.ResolveJSON(a.env.JSONOverride(cmd, opts.Options))
		if err != nil {
			return err
		}
	}
	ndjson := a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut)
	response, searchErr := a.reverseSearch(cmd.Context(), ReverseSearchRequest{
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
		return "", i18n.Errorf("error.provider_must_be_one_of_saucenao_ascii2d_color")
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
		return i18n.Errorf("error.ndjson_cannot_be_used_with_json")
	}
	for _, name := range []string{
		"search-by", "sort", "period", "start-date", "end-date", "rating", "type", "content-type",
		"resolution", "aspect-ratio", "draw-tool", "ai-mode", "bookmark-min", "bookmark-max",
		"bookmark-strategy", "limit", "page",
	} {
		if cmd.Flags().Changed(name) {
			return i18n.Errorf("error.is_not_supported_for_image_sources", name)
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

func (a commandRunner) writeReverseSearchResponse(response reversesearch.Response, jsonOut, ndjson bool) error {
	records, err := reverseSearchRecords(response.Results)
	if err != nil {
		return err
	}
	if ndjson {
		encoder := json.NewEncoder(a.env.Out)
		for _, item := range records {
			if err := encoder.Encode(item); err != nil {
				return err
			}
		}
		return nil
	}
	if jsonOut {
		return a.env.PrintJSON(reverseSearchOutput{
			Input: response.Input, Providers: nonNil(response.Providers), Results: nonNil(response.Results),
			Records: records, ProviderErrors: nonNil(response.ProviderErrors), Partial: response.Partial,
		})
	}
	return printReverseSearchHuman(a.env.Out, a.env.Language, response.Results)
}

func reverseSearchRecords(results []reversesearch.Result) ([]record.Record, error) {
	records := make([]record.Record, 0, len(results))
	for _, result := range results {
		if result.Pixiv == nil {
			continue
		}
		typeName, identityErr := reverseSearchIdentity(result.Pixiv)
		if identityErr != nil {
			return nil, identityErr
		}
		identity, err := record.NewIdentityRecord(result.Pixiv.ID, typeName)
		if err != nil {
			return nil, i18n.Errorf("error.reverse_search_returned_an_invalid_pixiv_identity")
		}
		records = append(records, identity)
	}
	return records, nil
}

func reverseSearchIdentity(ref *reversesearch.PixivRef) (recordType string, err error) {
	if ref == nil {
		return "", i18n.Errorf("error.reverse_search_returned_an_invalid_pixiv_identity")
	}
	switch ref.Type {
	case reversesearch.PixivRefUser:
		return string(target.NamespaceUser), nil
	case reversesearch.PixivRefArtwork:
		return string(target.NamespaceArtwork), nil
	default:
		return "", i18n.Errorf("error.reverse_search_returned_an_invalid_pixiv_identity")
	}
}

func printReverseSearchHuman(out io.Writer, language i18n.Locale, results []reversesearch.Result) error {
	for _, result := range results {
		line := language.Text("reverse.external")
		if result.Pixiv != nil {
			typeName, identityErr := reverseSearchIdentity(result.Pixiv)
			if identityErr != nil {
				return identityErr
			}
			identity, err := record.NewIdentityRecord(result.Pixiv.ID, typeName)
			if err != nil {
				return i18n.Errorf("error.reverse_search_returned_an_invalid_pixiv_identity")
			}
			line = identity.URL()
		}
		if title := display.SafeLine(result.Title); title != "" {
			line += " — " + title
		}
		if author := display.SafeLine(result.Author); author != "" {
			line += language.Text("reverse.by") + author
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func (a commandRunner) writeReverseSearchWarning(response reversesearch.Response) error {
	failed := make([]string, 0, len(response.ProviderErrors))
	for _, providerError := range response.ProviderErrors {
		failed = append(failed, string(providerError.Provider))
	}
	if len(failed) == 0 {
		failed = append(failed, a.env.Text("reverse.unknown_provider"))
	}
	_, err := fmt.Fprintf(a.env.Err, a.env.Text("output.warning_reverse_search_completed_partially_failed_providers"), strings.Join(failed, ", "))
	return err
}

// nonNil 让空 JSON 数组序列化为 []，避免输出 null。
func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
