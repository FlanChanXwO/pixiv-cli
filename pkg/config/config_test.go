package config

import "testing"

func TestLoadFromEnvDefaults(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "")
	t.Setenv("DOWNLOAD_PATH", "")
	t.Setenv("FILENAME_TEMPLATE", "")
	t.Setenv("https_proxy", "")

	cfg := LoadFromEnv()
	if cfg.DownloadPath != DefaultDownloadPath {
		t.Fatalf("DownloadPath = %q, want %q", cfg.DownloadPath, DefaultDownloadPath)
	}
	if cfg.FilenameTemplate != DefaultFilenameTemplate {
		t.Fatalf("FilenameTemplate = %q, want %q", cfg.FilenameTemplate, DefaultFilenameTemplate)
	}
}

func TestLoadFromEnvValues(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "refresh")
	t.Setenv("DOWNLOAD_PATH", "/tmp/pixiv")
	t.Setenv("FILENAME_TEMPLATE", "{id}")
	t.Setenv("https_proxy", "http://127.0.0.1:7890")

	cfg := LoadFromEnv()
	if cfg.RefreshToken != "refresh" || cfg.DownloadPath != "/tmp/pixiv" || cfg.FilenameTemplate != "{id}" || cfg.HTTPSProxy == "" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadFromEnvExtractsRefreshTokenFromCookie(t *testing.T) {
	t.Setenv("PIXIV_REFRESH_TOKEN", "PHPSESSID=web-session; refresh_token=refresh%2Ftoken; device_token=device")

	cfg := LoadFromEnv()
	if cfg.RefreshToken != "refresh/token" {
		t.Fatalf("RefreshToken = %q", cfg.RefreshToken)
	}
}
