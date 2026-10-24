package settings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
)

import (
	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
	"github.com/creachadair/tomledit/transform"
)

func loadConfigDocumentWithFileStore(path string, store FileStore) (*tomledit.Document, error) {
	store, err := requireFileStore(store)
	if err != nil {
		return nil, err
	}
	body, err := store.ReadFile(path)
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

func saveConfigDocumentWithFileStore(path string, doc *tomledit.Document, store FileStore) error {
	store, err := requireFileStore(store)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tomledit.Format(&buf, doc); err != nil {
		return err
	}
	return store.WritePrivateFile(path, buf.Bytes())
}

// setConfigValue 在注入的文件端口上执行稀疏配置写回。
func setConfigValue(path string, key Key, value parser.Value, store FileStore) error {
	store, err := requireFileStore(store)
	if err != nil {
		return err
	}
	spec, ok := SettingSpecByAlias(key)
	if !ok {
		return fmt.Errorf("unknown config key %q", key)
	}
	doc, err := loadConfigDocumentWithFileStore(path, store)
	if err != nil {
		return err
	}
	section := ensureConfigSection(doc, spec.Table)
	inserted := transform.InsertMapping(section, &parser.KeyValue{
		Name:  parser.Key{spec.Key},
		Value: value,
	}, true)
	if !inserted {
		return fmt.Errorf("failed to update config key %q", key)
	}
	return saveConfigDocumentWithFileStore(path, doc, store)
}

// unsetConfigValue 在注入的文件端口上删除一个配置键。
func unsetConfigValue(path string, key Key, store FileStore) (bool, error) {
	store, err := requireFileStore(store)
	if err != nil {
		return false, err
	}
	spec, ok := SettingSpecByAlias(key)
	if !ok {
		return false, fmt.Errorf("unknown config key %q", key)
	}
	doc, err := loadConfigDocumentWithFileStore(path, store)
	if err != nil {
		return false, err
	}
	entry := doc.First(append(spec.Table, spec.Key)...)
	if entry == nil {
		return false, saveConfigDocumentWithFileStore(path, doc, store)
	}
	removed := entry.Remove()
	if sectionEntry := transform.FindTable(doc, spec.Table...); sectionEntry != nil && len(sectionEntry.Section.Items) == 0 {
		sectionEntry.Remove()
	}
	return removed, saveConfigDocumentWithFileStore(path, doc, store)
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
