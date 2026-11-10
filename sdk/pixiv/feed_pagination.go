package pixiv

import (
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
)

func nonnegativeOffset(values []string) bool {
	if len(values) != 1 {
		return false
	}
	n, err := strconv.ParseInt(values[0], 10, 64)
	return err == nil && n >= 0
}

func validRecommendationPosition(path string, values url.Values) bool {
	if len(values) == 0 {
		return false
	}
	for key, entries := range values {
		if len(entries) != 1 || entries[0] == "" {
			return false
		}
		switch key {
		case api.ParamOffset:
			if !nonnegativeOffset(entries) {
				return false
			}
		case api.ParamMaxRecommend:
			if !positiveValues(entries, true) {
				return false
			}
		case api.ParamMinRecentIllusts:
			if path != api.PathIllustRecommended || !positiveValues(entries, true) {
				return false
			}
		case api.ParamAlreadyRecommended:
			if path != api.PathNovelRecommended {
				return false
			}
		case api.ParamIncludePrivacy, api.ParamIncludeRanking, api.ParamIncludeRankingNovels:
			if key == api.ParamIncludeRanking && path != api.PathIllustRecommended || key == api.ParamIncludeRankingNovels && path != api.PathNovelRecommended || entries[0] != "true" && entries[0] != "false" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
