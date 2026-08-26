package reversesearch

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type pixivRefKey struct {
	typeName PixivRefType
	id       int64
}

func appendProviderMatches(results *[]Result, canonical map[pixivRefKey]int, provider Provider, matches []Match, pixivOnly bool) {
	ordered := append([]Match(nil), matches...)
	sort.SliceStable(ordered, func(left, right int) bool { return ordered[left].Rank < ordered[right].Rank })
	for _, match := range ordered {
		pixiv := pixivRefFromMatch(match)
		if pixivOnly && pixiv == nil {
			continue
		}
		evidence := evidenceFromMatch(provider, match)
		if pixiv == nil {
			*results = append(*results, Result{Title: match.Title, Author: match.Author, Evidence: []Evidence{evidence}})
			continue
		}
		key := pixivRefKey{typeName: pixiv.Type, id: pixiv.ID}
		if index, exists := canonical[key]; exists {
			(*results)[index].Evidence = append((*results)[index].Evidence, evidence)
			continue
		}
		canonical[key] = len(*results)
		*results = append(*results, Result{
			Pixiv: pixiv, Title: match.Title, Author: match.Author, Evidence: []Evidence{evidence},
		})
	}
}

func pixivRefFromMatch(match Match) *PixivRef {
	if match.ArtworkID > 0 {
		return &PixivRef{Type: PixivRefArtwork, ID: match.ArtworkID}
	}
	var user *PixivRef
	for _, externalURL := range match.ExternalURLs {
		candidate := parsePixivURL(externalURL)
		if candidate == nil {
			continue
		}
		if candidate.Type == PixivRefArtwork {
			return candidate
		}
		if user == nil {
			user = candidate
		}
	}
	if match.UserID > 0 {
		return &PixivRef{Type: PixivRefUser, ID: match.UserID}
	}
	return user
}

func parsePixivURL(raw string) *PixivRef {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" ||
		!strings.EqualFold(parsed.Hostname(), "www.pixiv.net") || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.RawPath != "" {
		return nil
	}
	segments := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(segments) != 2 || segments[1] == "" || segments[1][0] == '0' {
		return nil
	}
	id, err := strconv.ParseInt(segments[1], 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != segments[1] {
		return nil
	}
	switch segments[0] {
	case "artworks":
		return &PixivRef{Type: PixivRefArtwork, ID: id}
	case "users":
		return &PixivRef{Type: PixivRefUser, ID: id}
	default:
		return nil
	}
}

func evidenceFromMatch(provider Provider, match Match) Evidence {
	return Evidence{
		Provider: provider, Rank: match.Rank, Similarity: match.Similarity,
		IndexID: match.IndexID, IndexName: match.IndexName, Title: match.Title, Author: match.Author,
		ExternalURLs: append([]string(nil), match.ExternalURLs...),
	}
}

func cloneQuota(quota *Quota) *Quota {
	if quota == nil {
		return nil
	}
	copy := *quota
	return &copy
}
