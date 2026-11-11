package saucenao

import (
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const (
	pixivHost        = "www.pixiv.net"
	pixivArtworkPath = "artworks"
)

type flexibleInt int64

// UnmarshalJSON accepts the integer and quoted-integer forms used by SauceNAO.
func (value *flexibleInt) UnmarshalJSON(raw []byte) error {
	parsed, err := strconv.ParseInt(strings.Trim(string(raw), `"`), 10, 64)
	if err != nil {
		return errors.New("invalid integer in SauceNAO response")
	}
	*value = flexibleInt(parsed)
	return nil
}

type flexibleFloat float64

// UnmarshalJSON accepts finite numeric and quoted-numeric similarity values.
func (value *flexibleFloat) UnmarshalJSON(raw []byte) error {
	parsed, err := strconv.ParseFloat(strings.Trim(string(raw), `"`), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return errors.New("invalid similarity in SauceNAO response")
	}
	*value = flexibleFloat(parsed)
	return nil
}

type wireResponse struct {
	Header  *wireHeader  `json:"header"`
	Results *[]wireMatch `json:"results"`
}

type wireHeader struct {
	Status         *flexibleInt `json:"status"`
	ShortRemaining *flexibleInt `json:"short_remaining"`
	LongRemaining  *flexibleInt `json:"long_remaining"`
	ShortLimit     *flexibleInt `json:"short_limit"`
	LongLimit      *flexibleInt `json:"long_limit"`
}

type wireMatch struct {
	Header *struct {
		Similarity *flexibleFloat `json:"similarity"`
		IndexID    *flexibleInt   `json:"index_id"`
		IndexName  string         `json:"index_name"`
	} `json:"header"`
	Data *struct {
		ExternalURLs []string    `json:"ext_urls"`
		Title        string      `json:"title"`
		PixivID      flexibleInt `json:"pixiv_id"`
		MemberName   string      `json:"member_name"`
		AuthorName   string      `json:"author_name"`
		MemberID     flexibleInt `json:"member_id"`
	} `json:"data"`
}

func decodeResponse(body io.Reader) (Result, error) {
	malformed := func(cause error) (Result, error) {
		return Result{}, failure(sdk.MalformedUpstreamResponse, "SauceNAO returned a malformed response", cause)
	}
	decoder := json.NewDecoder(body)
	var wire wireResponse
	if err := decoder.Decode(&wire); err != nil {
		return malformed(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("unexpected data after SauceNAO response")
		}
		return malformed(err)
	}
	if wire.Header == nil || wire.Header.Status == nil {
		return malformed(errors.New("missing response header or status"))
	}
	header := wire.Header
	result := Result{}
	if header.ShortRemaining != nil && header.LongRemaining != nil && header.ShortLimit != nil && header.LongLimit != nil {
		result.Quota = &Quota{ShortRemaining: int64(*header.ShortRemaining), LongRemaining: int64(*header.LongRemaining), ShortLimit: int64(*header.ShortLimit), LongLimit: int64(*header.LongLimit)}
	}
	if *header.Status != 0 {
		reason := sdk.UpstreamError
		if (header.ShortRemaining != nil && *header.ShortRemaining <= 0) || (header.LongRemaining != nil && *header.LongRemaining <= 0) {
			reason = sdk.RateLimited
		}
		return result, failure(reason, "SauceNAO rejected the query", nil)
	}
	if result.Quota == nil || wire.Results == nil {
		return malformed(errors.New("missing response quota or results"))
	}
	result.Matches = make([]Match, 0, len(*wire.Results))
	for index, match := range *wire.Results {
		if match.Header == nil || match.Data == nil || match.Header.Similarity == nil || match.Header.IndexID == nil || match.Header.IndexName == "" {
			return malformed(errors.New("missing result fields"))
		}
		artworkID := int64(match.Data.PixivID)
		if artworkID == 0 {
			artworkID = pixivArtworkID(match.Data.ExternalURLs)
		}
		result.Matches = append(result.Matches, Match{
			Rank: index + 1, Similarity: float64(*match.Header.Similarity), IndexID: int64(*match.Header.IndexID), IndexName: match.Header.IndexName,
			Title: match.Data.Title, Author: cmp.Or(match.Data.MemberName, match.Data.AuthorName), ArtworkID: artworkID, UserID: int64(match.Data.MemberID), ExternalURLs: match.Data.ExternalURLs,
		})
	}
	return result, nil
}

func pixivArtworkID(links []string) int64 {
	for _, link := range links {
		parsed, err := url.Parse(link)
		if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), pixivHost) || parsed.Port() != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawPath != "" {
			continue
		}
		segments := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
		if len(segments) != 2 || segments[0] != pixivArtworkPath {
			continue
		}
		id, err := strconv.ParseInt(segments[1], 10, 64)
		if err == nil && id > 0 && strconv.FormatInt(id, 10) == segments[1] {
			return id
		}
	}
	return 0
}
