package settings

// Key 指定配置别名，与 RuntimeConfig 字段上的 alias 标签一一对应。
type Key string

// 以下 Key 是 config.toml 的扁平别名，与 [RuntimeConfig] 字段上的 alias 标签一一对应；
// 键的含义见对应字段的注释，[Store.Set] 与 [Store.Get] 只接受这里列出的值。
const (
	KeyAccountPoolEnabled                 Key = "account_pool_enabled"
	KeyAccountPoolStrategy                Key = "account_pool_strategy"
	KeyDirectoryTemplate                  Key = "directory_template"
	KeyDownloadPath                       Key = "download_path"
	KeyFilenameTemplate                   Key = "filename_template"
	KeyHTTPSProxy                         Key = "https_proxy"
	KeyLogFormat                          Key = "log_format"
	KeyLogLevel                           Key = "log_level"
	KeyLoginOpenBrowser                   Key = "login_open_browser"
	KeyLoginRelayListenAddr               Key = "login_relay_listen_addr"
	KeyLoginRelayPublicURL                Key = "login_relay_public_url"
	KeyLoginRelayTLSCertFile              Key = "login_relay_tls_cert_file"
	KeyLoginRelayTLSKeyFile               Key = "login_relay_tls_key_file"
	KeyLoginUseAfterLogin                 Key = "login_use_after_login"
	KeyMCPGalleryAnimationTempCapacityMiB Key = "mcp_gallery_animation_temp_capacity_mib"
	KeyMCPGalleryCacheCapacityMiB         Key = "mcp_gallery_cache_capacity_mib"
	KeyOutputJSON                         Key = "output_json"
	KeyRequestInterval                    Key = "request_interval"
	KeyReverseSearchPixivOnly             Key = "reverse_search_pixiv_only"
	KeyReverseSearchProvider              Key = "reverse_search_provider"
	KeySauceNAOAPIKey                     Key = "saucenao_api_key"
	KeyUpdateCheckEnabled                 Key = "update_check_enabled"
)
