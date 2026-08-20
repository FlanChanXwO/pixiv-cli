package e2e

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/system"
)

const (
	nativeBrowserE2EEnabledEnv = "BROWSER_NATIVE_E2E"
	nativeBrowserListEnv       = "BROWSER_NATIVE_BROWSERS"
)

// TestRealNativeBrowserProvider 是显式开启的 host evidence test，只从指定 profile
// 读取 allowlisted FANBOXSESSID；value、path 和 database 内容绝不进入测试输出。
func TestRealNativeBrowserProvider(t *testing.T) {
	if os.Getenv(nativeBrowserE2EEnabledEnv) != "1" {
		t.Skip("set BROWSER_NATIVE_E2E=1 to run native browser provider evidence")
	}

	browsers, err := nativeBrowserNames(os.Getenv(nativeBrowserListEnv))
	if err != nil {
		t.Fatal(err)
	}
	for _, browser := range browsers {
		browser := browser
		t.Run(browser, func(t *testing.T) {
			provider, err := system.New(browser)
			if err != nil {
				t.Fatalf("create %s provider: %v", browser, err)
			}
			defer func() {
				if err := provider.Close(); err != nil {
					t.Errorf("close %s provider: %v", browser, err)
				}
			}()

			profiles, err := provider.DiscoverProfiles(t.Context())
			if err != nil {
				t.Fatalf("discover %s profiles: %v", browser, err)
			}
			ids := make([]string, 0, len(profiles))
			for _, profile := range profiles {
				ids = append(ids, profile.ID)
			}
			sort.Strings(ids)
			t.Logf("discovered %s profile IDs: %s", browser, strings.Join(ids, ","))

			profileID := os.Getenv(nativeBrowserProfileEnv(browser))
			profile, err := system.SelectProfile(profiles, profileID)
			if err != nil {
				t.Fatalf("select %s profile: %v", browser, err)
			}

			secrets, err := provider.Read(t.Context(), system.DefaultQuery, profile.ID)
			if err != nil {
				t.Fatalf("read %s FANBOX session: %v", browser, err)
			}
			if len(secrets) != 1 || strings.TrimSpace(secrets[0].Value()) == "" {
				t.Fatalf("read %s profile %q returned %d empty or multiple allowlisted cookies", browser, profile.ID, len(secrets))
			}
			t.Logf("%s profile %q returned exactly one non-empty allowlisted cookie", browser, profile.ID)
		})
	}
}

func nativeBrowserNames(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 || (len(parts) == 1 && strings.TrimSpace(parts[0]) == "") {
		return nil, fmt.Errorf("%s is required when %s=1", nativeBrowserListEnv, nativeBrowserE2EEnabledEnv)
	}
	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		browser := strings.ToLower(strings.TrimSpace(part))
		if browser != "chrome" && browser != "edge" && browser != "firefox" && browser != "safari" {
			return nil, fmt.Errorf("%s contains unsupported browser %q", nativeBrowserListEnv, browser)
		}
		if _, ok := seen[browser]; ok {
			return nil, fmt.Errorf("%s contains duplicate browser %q", nativeBrowserListEnv, browser)
		}
		seen[browser] = struct{}{}
		result = append(result, browser)
	}
	return result, nil
}

func nativeBrowserProfileEnv(browser string) string {
	return "BROWSER_NATIVE_PROFILE_" + strings.ToUpper(browser)
}

func TestNativeBrowserNamesRejectInvalidInput(t *testing.T) {
	for _, raw := range []string{"", "chrome,chrome", "chrome,unknown", "chrome,\x7f"} {
		if _, err := nativeBrowserNames(raw); err == nil {
			t.Fatalf("nativeBrowserNames(%q) unexpectedly succeeded", raw)
		}
	}
	if _, err := nativeBrowserNames("edge,chrome,safari"); err != nil {
		t.Fatalf("nativeBrowserNames(valid) = %v", err)
	}
}
