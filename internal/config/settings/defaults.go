package settings

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
)

var defaultConfigHeader = parser.Comments{
	"pixiv-cli configuration",
	`Use "pixiv config set KEY VALUE" to change a setting.`,
}

// baselineSectionOrder 固定首次生成的精简 config.toml 的 section 呈现顺序。
//
// 元数据的顺序来自 RuntimeConfig 的**字段声明顺序**（那是绑定的自然顺序），而
// 文件呈现顺序是一个独立的产品决定：先给用户看最常改的 download，再依次是
// output/login/update/logging/reverse_search。两者不应互相绑架，因此这里显式
// 列出顺序，而不是依赖字段声明或字母排序。
var baselineSectionOrder = []string{
	"download",
	"output",
	"login",
	"update",
	"logging",
	"reverse_search",
}

// baselineSectionRank 返回 section 在呈现顺序中的位置；未列出的 section 排在
// 已列出项之后（按名称稳定排序由调用方的排序保证）。
func baselineSectionRank(table []string) (int, bool) {
	name := joinTableName(table)
	for index, ordered := range baselineSectionOrder {
		if ordered == name {
			return index, true
		}
	}
	return len(baselineSectionOrder), false
}

func generatedDefaultConfig() ([]byte, error) {
	doc := &tomledit.Document{
		Global: &tomledit.Section{Items: []parser.Item{defaultConfigHeader}},
	}
	sections := make(map[string]*tomledit.Section)

	// 按固定呈现顺序遍历 section，保持与历史精简文件一致的用户可见布局。
	entries := make([]settingSpecFromTags, 0, len(mustSettingSpecs()))
	for _, entry := range mustSettingSpecs() {
		if entry.spec.Removed || !entry.spec.DefaultInFile {
			continue
		}
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		ri, _ := baselineSectionRank(entries[i].spec.Table)
		rj, _ := baselineSectionRank(entries[j].spec.Table)
		return ri < rj
	})

	for _, entry := range entries {
		spec := entry.spec
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
