package settings

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
)

var defaultConfigHeader = parser.Comments{
	"pixiv-cli configuration",
	`Use "pixiv config set KEY VALUE" to change a setting.`,
}

func generatedDefaultConfig() ([]byte, error) {
	doc := &tomledit.Document{
		Global: &tomledit.Section{Items: []parser.Item{defaultConfigHeader}},
	}
	sections := make(map[string]*tomledit.Section)

	for _, spec := range settingSpecs {
		if spec.Removed || !spec.DefaultInFile {
			continue
		}
		if !spec.HasDefault {
			return nil, fmt.Errorf("default config key %q has no default value", spec.Alias)
		}
		raw, err := defaultSettingInput(spec)
		if err != nil {
			return nil, err
		}
		_, value, err := ParseSettingInput(spec.Alias, raw)
		if err != nil {
			return nil, fmt.Errorf("generate default config key %q: %w", spec.Alias, err)
		}

		sectionName := joinTableName(spec.Table)
		section := sections[sectionName]
		if section == nil {
			section = &tomledit.Section{Heading: &parser.Heading{Name: append(parser.Key(nil), spec.Table...)}}
			sections[sectionName] = section
			doc.Sections = append(doc.Sections, section)
		}
		section.Items = append(section.Items, &parser.KeyValue{
			Name:  parser.Key{spec.Key},
			Value: value,
		})
	}

	var body bytes.Buffer
	if err := tomledit.Format(&body, doc); err != nil {
		return nil, fmt.Errorf("format generated default config: %w", err)
	}
	return body.Bytes(), nil
}

func defaultSettingInput(spec SettingSpec) (string, error) {
	switch value := spec.Default.(type) {
	case string:
		return value, nil
	case bool:
		return strconv.FormatBool(value), nil
	case time.Duration:
		return value.String(), nil
	default:
		return "", fmt.Errorf("default config key %q has unsupported default type %T", spec.Alias, spec.Default)
	}
}

func joinTableName(table []string) string {
	name := ""
	for index, part := range table {
		if index > 0 {
			name += "."
		}
		name += part
	}
	return name
}
