package e2e

import (
	"strings"
	"testing"
)

func TestAuthenticatedCanarySearchWordsFromEnvironment(t *testing.T) {
	t.Parallel()

	words, err := authenticatedCanarySearchWordsFromEnvironment(func(name string) string {
		switch name {
		case "PIXIV_E2E_ILLUST_SEARCH_WORD":
			return " 初音ミク "
		case "PIXIV_E2E_DISCOVERY_WORD":
			return " miku "
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("authenticatedCanarySearchWordsFromEnvironment() error = %v", err)
	}
	if words.illust != "初音ミク" || words.discovery != "miku" {
		t.Fatalf("search words = %+v", words)
	}
}

func TestAuthenticatedCanarySearchWordsFromEnvironmentRequiresEveryWord(t *testing.T) {
	t.Parallel()

	for _, missing := range []string{"PIXIV_E2E_ILLUST_SEARCH_WORD", "PIXIV_E2E_DISCOVERY_WORD"} {
		t.Run(missing, func(t *testing.T) {
			t.Parallel()
			_, err := authenticatedCanarySearchWordsFromEnvironment(func(name string) string {
				if name == missing {
					return " \t "
				}
				return "available"
			})
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("missing %s error = %v", missing, err)
			}
		})
	}
}

func TestAuthenticatedCanarySFWIllustIDFromEnvironment(t *testing.T) {
	t.Parallel()

	id, err := authenticatedCanarySFWIllustIDFromEnvironment(func(string) string { return " 147502481 " })
	if err != nil {
		t.Fatalf("authenticatedCanarySFWIllustIDFromEnvironment() error = %v", err)
	}
	if id != 147502481 {
		t.Fatalf("SFW illustration ID = %d", id)
	}

	for _, raw := range []string{"", "0", "-1", "not-an-id"} {
		_, err := authenticatedCanarySFWIllustIDFromEnvironment(func(string) string { return raw })
		if err == nil || !strings.Contains(err.Error(), "PIXIV_E2E_SFW_ILLUST_ID") {
			t.Fatalf("SFW illustration ID %q error = %v", raw, err)
		}
	}
}
