package e2e

import (
	"fmt"
	"strconv"
	"strings"
)

type authenticatedCanarySearchWords struct {
	illust    string
	discovery string
}

// authenticatedCanarySearchWordsFromEnvironment 只接受发布或本地调用者显式提供的
// 搜索输入，避免真实 E2E 将可能失效的热门标签固化为源码实现细节。
func authenticatedCanarySearchWordsFromEnvironment(lookup func(string) string) (authenticatedCanarySearchWords, error) {
	if lookup == nil {
		return authenticatedCanarySearchWords{}, fmt.Errorf("authenticated canary environment lookup is required")
	}
	words := authenticatedCanarySearchWords{
		illust:    strings.TrimSpace(lookup("PIXIV_E2E_ILLUST_SEARCH_WORD")),
		discovery: strings.TrimSpace(lookup("PIXIV_E2E_DISCOVERY_WORD")),
	}
	if words.illust == "" {
		return authenticatedCanarySearchWords{}, fmt.Errorf("PIXIV_E2E_ILLUST_SEARCH_WORD is required when PIXIV_E2E_REAL_API=1")
	}
	if words.discovery == "" {
		return authenticatedCanarySearchWords{}, fmt.Errorf("PIXIV_E2E_DISCOVERY_WORD is required when PIXIV_E2E_REAL_API=1")
	}
	return words, nil
}

func authenticatedCanarySFWIllustIDFromEnvironment(lookup func(string) string) (int64, error) {
	if lookup == nil {
		return 0, fmt.Errorf("authenticated canary environment lookup is required")
	}
	const name = "PIXIV_E2E_SFW_ILLUST_ID"
	raw := strings.TrimSpace(lookup(name))
	if raw == "" {
		return 0, fmt.Errorf("%s is required when PIXIV_E2E_REAL_API=1", name)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a positive illustration ID", name)
	}
	return id, nil
}
