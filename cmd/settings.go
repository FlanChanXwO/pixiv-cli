package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-mcp-server/pkg/pixivutil"
	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
	"github.com/creachadair/tomledit/transform"
	"github.com/knadh/koanf/parsers/toml/v2"
	koanfenv "github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	defaultDownloadPath     = "./downloads"
	defaultFilenameTemplate = "{author} - {title}_{id}"
)

type settingKind string

const (
	settingString   settingKind = "string"
	settingBool     settingKind = "bool"
	settingDuration settingKind = "duration"
)

type settingSpec struct {
	Alias      string
	KoanfKey   string
	Table      []string
	Key        string
	Kind       settingKind
	HasDefault bool
	Default    any
}

type settingValue struct {
	Value    any
	Text     string
	Source   string
	HasValue bool
}

type settingsState struct {
	file   *koanf.Koanf
	merged *koanf.Koanf
}

type runtimeConfig struct {
	RefreshToken       string
	DownloadPath       string
	FilenameTemplate   string
	HTTPSProxy         string
	OutputJSON         bool
	LoginOpenBrowser   bool
	LoginTimeout       time.Duration
	LoginUseAfterLogin bool
}

var settingSpecs = []settingSpec{
	{Alias: "download_path", KoanfKey: "download.path", Table: []string{"download"}, Key: "path", Kind: settingString, HasDefault: true, Default: defaultDownloadPath},
	{Alias: "filename_template", KoanfKey: "download.filename_template", Table: []string{"download"}, Key: "filename_template", Kind: settingString, HasDefault: true, Default: defaultFilenameTemplate},
	{Alias: "https_proxy", KoanfKey: "network.https_proxy", Table: []string{"network"}, Key: "https_proxy", Kind: settingString},
	{Alias: "output_json", KoanfKey: "output.json", Table: []string{"output"}, Key: "json", Kind: settingBool, HasDefault: true, Default: false},
	{Alias: "login_open_browser", KoanfKey: "login.open_browser", Table: []string{"login"}, Key: "open_browser", Kind: settingBool, HasDefault: true, Default: true},
	{Alias: "login_timeout", KoanfKey: "login.timeout", Table: []string{"login"}, Key: "timeout", Kind: settingDuration, HasDefault: true, Default: time.Duration(0)},
	{Alias: "login_use_after_login", KoanfKey: "login.use_after_login", Table: []string{"login"}, Key: "use_after_login", Kind: settingBool, HasDefault: true, Default: false},
}

func settingSpecByAlias(alias string) (settingSpec, bool) {
	for _, spec := range settingSpecs {
		if spec.Alias == alias {
			return spec, true
		}
	}
	return settingSpec{}, false
}

func validSettingAliases() []string {
	keys := make([]string, 0, len(settingSpecs))
	for _, spec := range settingSpecs {
		keys = append(keys, spec.Alias)
	}
	slices.Sort(keys)
	return keys
}

func refreshTokenFromEnv() string {
	token, _ := pixivutil.ParseRefreshTokenInput(os.Getenv("PIXIV_REFRESH_TOKEN"))
	return token
}

func envValue(spec settingSpec) (string, bool) {
	switch spec.Alias {
	case "download_path":
		return envLookup("DOWNLOAD_PATH")
	case "filename_template":
		return envLookup("FILENAME_TEMPLATE")
	case "https_proxy":
		if value, ok := envLookup("https_proxy"); ok {
			return value, true
		}
		return envLookup("HTTPS_PROXY")
	default:
		return "", false
	}
}

func envLookup(name string) (string, bool) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", false
	}
	return value, true
}

func configEnvEntries() []string {
	out := make([]string, 0, 3)
	if value, ok := envLookup("DOWNLOAD_PATH"); ok {
		out = append(out, "DOWNLOAD_PATH="+value)
	}
	if value, ok := envLookup("FILENAME_TEMPLATE"); ok {
		out = append(out, "FILENAME_TEMPLATE="+value)
	}
	if value, ok := envLookup("https_proxy"); ok {
		out = append(out, "https_proxy="+value)
	} else if value, ok := envLookup("HTTPS_PROXY"); ok {
		out = append(out, "HTTPS_PROXY="+value)
	}
	return out
}

func loadSettingsState() (settingsState, error) {
	path, err := configFilePath()
	if err != nil {
		return settingsState{}, err
	}
	fileState := koanf.New(".")
	mergedState := koanf.New(".")
	if err := loadConfigFileInto(fileState, path); err != nil {
		return settingsState{}, err
	}
	if err := loadConfigFileInto(mergedState, path); err != nil {
		return settingsState{}, err
	}
	if err := mergedState.Load(koanfenv.Provider(".", koanfenv.Opt{
		EnvironFunc: configEnvEntries,
		TransformFunc: func(key, value string) (string, any) {
			switch key {
			case "DOWNLOAD_PATH":
				return "download.path", value
			case "FILENAME_TEMPLATE":
				return "download.filename_template", value
			case "https_proxy", "HTTPS_PROXY":
				return "network.https_proxy", value
			default:
				return "", nil
			}
		},
	}), nil); err != nil {
		return settingsState{}, err
	}
	return settingsState{file: fileState, merged: mergedState}, nil
}

func loadConfigFileInto(target *koanf.Koanf, path string) error {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	return target.Load(file.Provider(path), toml.Parser())
}

func (s settingsState) effective(alias string) (settingValue, error) {
	spec, ok := settingSpecByAlias(alias)
	if !ok {
		return settingValue{}, fmt.Errorf("unknown config key %q", alias)
	}
	if raw, ok := envValue(spec); ok {
		return coerceSettingValue(spec, raw, "env")
	}
	if s.file.Exists(spec.KoanfKey) {
		return coerceSettingValue(spec, s.file.Get(spec.KoanfKey), "file")
	}
	if spec.HasDefault {
		return coerceSettingValue(spec, spec.Default, "default")
	}
	return settingValue{Source: "unset"}, nil
}

func (s settingsState) runtime() (runtimeConfig, error) {
	downloadPath, err := s.effective("download_path")
	if err != nil {
		return runtimeConfig{}, err
	}
	filenameTemplate, err := s.effective("filename_template")
	if err != nil {
		return runtimeConfig{}, err
	}
	httpsProxy, err := s.effective("https_proxy")
	if err != nil {
		return runtimeConfig{}, err
	}
	outputJSON, err := s.effective("output_json")
	if err != nil {
		return runtimeConfig{}, err
	}
	loginOpenBrowser, err := s.effective("login_open_browser")
	if err != nil {
		return runtimeConfig{}, err
	}
	loginTimeout, err := s.effective("login_timeout")
	if err != nil {
		return runtimeConfig{}, err
	}
	loginUseAfterLogin, err := s.effective("login_use_after_login")
	if err != nil {
		return runtimeConfig{}, err
	}
	cfg := runtimeConfig{
		DownloadPath:       downloadPath.Value.(string),
		FilenameTemplate:   filenameTemplate.Value.(string),
		HTTPSProxy:         "",
		OutputJSON:         outputJSON.Value.(bool),
		LoginOpenBrowser:   loginOpenBrowser.Value.(bool),
		LoginTimeout:       loginTimeout.Value.(time.Duration),
		LoginUseAfterLogin: loginUseAfterLogin.Value.(bool),
	}
	if httpsProxy.HasValue {
		cfg.HTTPSProxy = httpsProxy.Value.(string)
	}
	return cfg, nil
}

func coerceSettingValue(spec settingSpec, raw any, source string) (settingValue, error) {
	switch spec.Kind {
	case settingString:
		text, ok := normalizeStringValue(raw)
		if !ok {
			return settingValue{}, fmt.Errorf("%s expects string, got %T", spec.Alias, raw)
		}
		if text == "" && !spec.HasDefault && source != "default" {
			return settingValue{Source: source}, nil
		}
		return settingValue{Value: text, Text: text, Source: source, HasValue: true}, nil
	case settingBool:
		value, err := normalizeBoolValue(raw)
		if err != nil {
			return settingValue{}, fmt.Errorf("%s: %w", spec.Alias, err)
		}
		return settingValue{Value: value, Text: strconv.FormatBool(value), Source: source, HasValue: true}, nil
	case settingDuration:
		value, err := normalizeDurationValue(raw)
		if err != nil {
			return settingValue{}, fmt.Errorf("%s: %w", spec.Alias, err)
		}
		return settingValue{Value: value, Text: value.String(), Source: source, HasValue: true}, nil
	default:
		return settingValue{}, fmt.Errorf("unsupported setting kind %q", spec.Kind)
	}
}

func normalizeStringValue(raw any) (string, bool) {
	switch value := raw.(type) {
	case string:
		return value, true
	case fmt.Stringer:
		return value.String(), true
	case nil:
		return "", false
	default:
		return fmt.Sprint(value), true
	}
}

func normalizeBoolValue(raw any) (bool, error) {
	switch value := raw.(type) {
	case bool:
		return value, nil
	case string:
		return strconv.ParseBool(strings.TrimSpace(value))
	default:
		return false, fmt.Errorf("expects bool, got %T", raw)
	}
}

func normalizeDurationValue(raw any) (time.Duration, error) {
	switch value := raw.(type) {
	case time.Duration:
		return value, nil
	case string:
		return time.ParseDuration(strings.TrimSpace(value))
	default:
		return 0, fmt.Errorf("expects duration string, got %T", raw)
	}
}

func parseSettingInput(alias, raw string) (settingValue, parser.Value, error) {
	spec, ok := settingSpecByAlias(alias)
	if !ok {
		return settingValue{}, parser.Value{}, fmt.Errorf("unknown config key %q", alias)
	}
	raw = strings.TrimSpace(raw)
	switch spec.Kind {
	case settingString:
		value, err := parser.ParseValue(strconv.Quote(raw))
		if err != nil {
			return settingValue{}, parser.Value{}, err
		}
		return settingValue{Value: raw, Text: raw, Source: "cli", HasValue: true}, value, nil
	case settingBool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return settingValue{}, parser.Value{}, fmt.Errorf("expects bool value: %w", err)
		}
		value, err := parser.ParseValue(strconv.FormatBool(parsed))
		if err != nil {
			return settingValue{}, parser.Value{}, err
		}
		return settingValue{Value: parsed, Text: strconv.FormatBool(parsed), Source: "cli", HasValue: true}, value, nil
	case settingDuration:
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return settingValue{}, parser.Value{}, fmt.Errorf("expects duration value: %w", err)
		}
		normalized := parsed.String()
		value, err := parser.ParseValue(strconv.Quote(normalized))
		if err != nil {
			return settingValue{}, parser.Value{}, err
		}
		return settingValue{Value: parsed, Text: normalized, Source: "cli", HasValue: true}, value, nil
	default:
		return settingValue{}, parser.Value{}, fmt.Errorf("unsupported setting kind %q", spec.Kind)
	}
}

func loadConfigDocument(path string) (*tomledit.Document, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &tomledit.Document{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return &tomledit.Document{}, nil
	}
	return tomledit.Parse(bytes.NewReader(body))
}

func saveConfigDocument(path string, doc *tomledit.Document) error {
	var buf bytes.Buffer
	if err := tomledit.Format(&buf, doc); err != nil {
		return err
	}
	return writePrivateFile(path, buf.Bytes())
}

func setConfigValue(path, alias string, value parser.Value) error {
	spec, ok := settingSpecByAlias(alias)
	if !ok {
		return fmt.Errorf("unknown config key %q", alias)
	}
	doc, err := loadConfigDocument(path)
	if err != nil {
		return err
	}
	section := ensureConfigSection(doc, spec.Table)
	inserted := transform.InsertMapping(section, &parser.KeyValue{
		Name:  parser.Key{spec.Key},
		Value: value,
	}, true)
	if !inserted {
		return fmt.Errorf("failed to update config key %q", alias)
	}
	return saveConfigDocument(path, doc)
}

func unsetConfigValue(path, alias string) (bool, error) {
	spec, ok := settingSpecByAlias(alias)
	if !ok {
		return false, fmt.Errorf("unknown config key %q", alias)
	}
	doc, err := loadConfigDocument(path)
	if err != nil {
		return false, err
	}
	entry := doc.First(append(spec.Table, spec.Key)...)
	if entry == nil {
		return false, saveConfigDocument(path, doc)
	}
	removed := entry.Remove()
	if sectionEntry := transform.FindTable(doc, spec.Table...); sectionEntry != nil && len(sectionEntry.Section.Items) == 0 {
		sectionEntry.Remove()
	}
	return removed, saveConfigDocument(path, doc)
}

func ensureConfigSection(doc *tomledit.Document, name []string) *tomledit.Section {
	if entry := transform.FindTable(doc, name...); entry != nil {
		return entry.Section
	}
	section := &tomledit.Section{
		Heading: &parser.Heading{Name: parser.Key(name)},
	}
	doc.Sections = append(doc.Sections, section)
	return section
}
