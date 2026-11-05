package config

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
	"github.com/creachadair/tomledit/transform"
	"github.com/pelletier/go-toml/v2"
)

// Get 返回目标键的有效值及存在性；配置中的 API key 原样返回。
func (s Store) Get(key string) (string, bool, error) {
	c := Default()
	p, err := field(&c, key)
	if err != nil {
		return "", false, err
	}
	if err := s.load(&c); err != nil {
		return "", false, err
	}
	value, present := text(p)
	if !present {
		return "", false, nil
	}
	value, err = validateValue(key, value)
	return value, true, err
}

// Set 按目标字段类型和取值规则校验输入，保留目标键注释与其他键。
func (s Store) Set(key, raw string) error {
	c := Default()
	p, err := field(&c, key)
	if err != nil {
		return err
	}
	raw, err = validateValue(key, raw)
	if err != nil {
		return err
	}
	var value any = raw
	switch p.(type) {
	case *bool:
		parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("%s expects bool: %w", key, err)
		}
		value = parsed
	case *int64, **int64:
		parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return fmt.Errorf("%s expects integer: %w", key, err)
		}
		value = parsed
	}
	encoded, err := toml.Marshal(map[string]any{"value": value})
	if err != nil {
		return err
	}
	doc, err := tomledit.Parse(bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	return s.edit(key, &doc.First("value").Value)
}

// Unset 删除目标 TOML 叶子路径，其他键保持原样。
func (s Store) Unset(key string) error {
	c := Default()
	if _, err := field(&c, key); err != nil {
		return err
	}
	return s.edit(key, nil)
}

func (s Store) edit(key string, value *parser.Value) error {
	body, err := s.readFile()
	if err != nil {
		return err
	}
	doc, err := tomledit.Parse(bytes.NewReader(body))
	if err != nil {
		return err
	}
	parts := strings.Split(key, ".")
	entry := doc.First(parts...)
	if value == nil {
		if entry != nil {
			entry.Remove()
		}
	} else if entry != nil {
		if entry.IsSection() {
			return fmt.Errorf("config key %q is a table", key)
		}
		// 只更新值，保留原有键的行尾与前置注释。
		entry.Value.X = value.X
	} else {
		if err := insert(doc, parts, *value); err != nil {
			return err
		}
	}
	var out bytes.Buffer
	if err := tomledit.Format(&out, doc); err != nil {
		return err
	}
	// tomledit 只检查语法；写入前还需拒绝重复表、重复键等 TOML 语义错误。
	var decoded map[string]any
	if err := toml.Unmarshal(out.Bytes(), &decoded); err != nil {
		return err
	}
	// 只校验被编辑的 solver 组，让其他配置错误仍可逐项修复。
	if parts[1] == "flaresolverr" {
		table, err := lookupTable(decoded, parts[:2]...)
		if err != nil {
			return err
		}
		encoded, err := toml.Marshal(table)
		if err != nil {
			return err
		}
		var solver FlareSolverrConfig
		if err := toml.Unmarshal(encoded, &solver); err != nil {
			return err
		}
		if err := validateSolver(strings.Join(parts[:2], "."), solver); err != nil {
			return err
		}
	}
	path, err := s.Path()
	if err != nil {
		return err
	}
	return s.Files.WritePrivateFile(path, out.Bytes())
}

func insert(doc *tomledit.Document, path []string, value parser.Value) error {
	for length := len(path) - 1; length > 0; length-- {
		parent := doc.First(path[:length]...)
		if parent == nil {
			continue
		}
		mapping := &parser.KeyValue{Name: parser.Key(path[length:]), Value: value}
		if parent.IsSection() {
			transform.InsertMapping(parent.Section, mapping, false)
			return nil
		}
		if table, ok := parent.Value.X.(parser.Inline); ok {
			parent.Value.X = append(table, mapping)
			return nil
		}
		return fmt.Errorf("config key %q is not a table", strings.Join(path[:length], "."))
	}
	// 没有显式父表时写入顶层点分键，兼容已有点分键定义的隐式表。
	if doc.Global == nil {
		doc.Global = new(tomledit.Section)
	}
	transform.InsertMapping(doc.Global, &parser.KeyValue{Name: parser.Key(path), Value: value}, false)
	return nil
}

func lookupTable(file map[string]any, path ...string) (map[string]any, error) {
	for index, key := range path {
		raw, present := file[key]
		if !present {
			return nil, nil
		}
		var ok bool
		file, ok = raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("config: %s must be a table", strings.Join(path[:index+1], "."))
		}
	}
	return file, nil
}
