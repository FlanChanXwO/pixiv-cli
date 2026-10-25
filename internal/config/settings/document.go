package settings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
	"github.com/creachadair/tomledit/transform"
)

func loadConfigDocumentWithFileStore(path string, store FileStore) (*tomledit.Document, error) {
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
	var buf bytes.Buffer
	if err := tomledit.Format(&buf, doc); err != nil {
		return err
	}
	return store.WritePrivateFile(path, buf.Bytes())
}

// setConfigValue 在注入的文件端口上执行稀疏配置写回。
func setConfigValue(path string, spec SettingSpec, value parser.Value, store FileStore) error {
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
		return fmt.Errorf("failed to update config key %q", spec.Alias)
	}
	return saveConfigDocumentWithFileStore(path, doc, store)
}

// unsetConfigValue 在注入的文件端口上删除一个配置键。
func unsetConfigValue(path string, spec SettingSpec, store FileStore) error {
	doc, err := loadConfigDocumentWithFileStore(path, store)
	if err != nil {
		return err
	}
	entry := doc.First(append(spec.Table, spec.Key)...)
	if entry == nil {
		return saveConfigDocumentWithFileStore(path, doc, store)
	}
	entry.Remove()
	if sectionEntry := transform.FindTable(doc, spec.Table...); sectionEntry != nil && len(sectionEntry.Section.Items) == 0 {
		sectionEntry.Remove()
	}
	return saveConfigDocumentWithFileStore(path, doc, store)
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
