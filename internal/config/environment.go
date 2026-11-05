package config

import "os"

func applyEnvironment(c *Config) {
	if v, ok := os.LookupEnv("DOWNLOAD_PATH"); ok {
		c.Download.Path = v
	}
	if v, ok := os.LookupEnv("FILENAME_TEMPLATE"); ok {
		c.Download.FilenameTemplate = v
	}
	if v, ok := os.LookupEnv("DIRECTORY_TEMPLATE"); ok {
		c.Download.DirectoryTemplate = v
	}
	if v, ok := os.LookupEnv("https_proxy"); ok {
		c.Network.HTTPSProxy = v
	} else if v, ok := os.LookupEnv("HTTPS_PROXY"); ok {
		c.Network.HTTPSProxy = v
	}
	if v, ok := os.LookupEnv("PIXIV_REQUEST_INTERVAL"); ok {
		c.Network.RequestInterval = v
	}
	if v, ok := os.LookupEnv("PIXIV_LOG_LEVEL"); ok {
		c.Logging.Level = LogLevel(v)
	}
	if v, ok := os.LookupEnv("PIXIV_LOG_FORMAT"); ok {
		c.Logging.Format = LogFormat(v)
	}
	if v, ok := os.LookupEnv("SAUCENAO_API_KEY"); ok {
		c.ReverseSearch.SauceNAOAPIKey = v
	}
}

// EnvironmentOverride 报告目标键是否受环境变量覆盖。
func EnvironmentOverride(key string) bool {
	var names []string
	switch key {
	case "download.path":
		names = []string{"DOWNLOAD_PATH"}
	case "download.filename_template":
		names = []string{"FILENAME_TEMPLATE"}
	case "download.directory_template":
		names = []string{"DIRECTORY_TEMPLATE"}
	case "network.https_proxy":
		names = []string{"https_proxy", "HTTPS_PROXY"}
	case "network.request_interval":
		names = []string{"PIXIV_REQUEST_INTERVAL"}
	case "logging.level":
		names = []string{"PIXIV_LOG_LEVEL"}
	case "logging.format":
		names = []string{"PIXIV_LOG_FORMAT"}
	case "reverse_search.saucenao_api_key":
		names = []string{"SAUCENAO_API_KEY"}
	}
	for _, name := range names {
		if _, ok := os.LookupEnv(name); ok {
			return true
		}
	}
	return false
}
