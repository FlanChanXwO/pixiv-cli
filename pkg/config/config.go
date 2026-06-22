package config

import (
	"os"

	"github.com/FlanChanXwO/pixiv-mcp-server/pkg/pixivutil"
)

const (
	DefaultDownloadPath     = "./downloads"
	DefaultFilenameTemplate = "{author} - {title}_{id}"
)

type Config struct {
	RefreshToken     string
	DownloadPath     string
	FilenameTemplate string
	HTTPSProxy       string
}

func LoadFromEnv() Config {
	cfg := Config{
		RefreshToken:     refreshTokenFromEnv(),
		DownloadPath:     getenv("DOWNLOAD_PATH", DefaultDownloadPath),
		FilenameTemplate: getenv("FILENAME_TEMPLATE", DefaultFilenameTemplate),
		HTTPSProxy:       os.Getenv("https_proxy"),
	}
	if cfg.HTTPSProxy == "" {
		cfg.HTTPSProxy = os.Getenv("HTTPS_PROXY")
	}
	return cfg
}

func refreshTokenFromEnv() string {
	token, _ := pixivutil.ParseRefreshTokenInput(os.Getenv("PIXIV_REFRESH_TOKEN"))
	return token
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
