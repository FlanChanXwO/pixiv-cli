package pipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Record 是可在 CLI 管道和 MCP 之间共享的 Pixiv 实体 JSON 记录。
// 它保留源对象和外部程序提供的未知字段，同时固定顶层 id、type、url。
type Record struct {
	fields map[string]any
}

var positiveIntegerJSON = regexp.MustCompile(`^[1-9][0-9]*$`)

// prohibitedRecordMetadataKeys 是记录协议明确禁止输出的版本/模式元数据键。
// 键会忽略大小写、下划线和连字符，以同时覆盖 api_version 与 apiVersion。
// 这里必须保持白名单，不能按包含 "version" 判断，避免误删 conversion 等业务字段。
var prohibitedRecordMetadataKeys = map[string]struct{}{
	"version": {}, "schema": {}, "apiversion": {}, "schemaversion": {}, "protocolversion": {},
	"formatversion": {}, "recordversion": {}, "sdkversion": {}, "mcpversion": {}, "cliversion": {}, "versioninfo": {},
}

// RecordFilter 描述基于稳定记录字段的本地筛选条件。
// MinViews 和 MinPageCount 为 nil 时不筛选对应字段。
type RecordFilter struct {
	ID           string
	Type         string
	Tags         []string
	MinViews     *int64
	MinPageCount *int64
}

// RecordFromArtworkDTO 将显式 DTO 映射为管道记录。运行时 SDK 模型必须先在
// 产品包内完成字段级转换，记录层不再通过反射猜测或脱敏运行时字段。
func RecordFromArtworkDTO(artwork pixiv.ArtworkDTO) (Record, error) {
	url := "https://www.pixiv.net/artworks/" + strconv.FormatInt(artwork.ID, 10)
	return recordFromDTO(artwork, artwork.ID, string(artwork.Kind), url)
}

// RecordFromNovelDTO 将小说 DTO 映射为管道记录。
func RecordFromNovelDTO(novel pixiv.NovelDTO) (Record, error) {
	url := "https://www.pixiv.net/novel/show.php?id=" + strconv.FormatInt(novel.ID, 10)
	return recordFromDTO(novel, novel.ID, "novel", url)
}

// RecordFromUserPreviewDTO 将用户预览 DTO 映射为管道记录。
func RecordFromUserPreviewDTO(preview pixiv.UserPreviewDTO) (Record, error) {
	id := strconv.FormatInt(preview.User.ID, 10)
	return recordFromDTO(preview, preview.User.ID, "user", "https://www.pixiv.net/users/"+id)
}

// RecordFromUserDetailDTO 将用户详情 DTO 映射为管道记录。
func RecordFromUserDetailDTO(detail pixiv.UserDetailDTO) (Record, error) {
	id := strconv.FormatInt(detail.User.ID, 10)
	return recordFromDTO(detail, detail.User.ID, "user", "https://www.pixiv.net/users/"+id)
}

// ID 返回记录的稳定字符串标识。
func (r Record) ID() string {
	return stringField(r.fields["id"])
}

// Type 返回记录的实体类型。
func (r Record) Type() string {
	return stringField(r.fields["type"])
}

// URL 返回记录的规范 Pixiv URL。
func (r Record) URL() string {
	return stringField(r.fields["url"])
}

// Matches 判断记录是否满足所有已设置的筛选条件，不修改记录内容。
func (r Record) Matches(filter RecordFilter) bool {
	if filter.ID != "" && r.ID() != filter.ID {
		return false
	}
	if filter.Type != "" && r.Type() != filter.Type {
		return false
	}
	if len(filter.Tags) > 0 && !recordHasTags(r.fields["tags"], filter.Tags) {
		return false
	}
	if filter.MinViews != nil {
		views, ok := recordInt(r.fields["total_view"])
		if !ok {
			views, ok = recordInt(r.fields["total_views"])
		}
		if !ok {
			views, ok = recordInt(r.fields["views"])
		}
		if !ok || views < *filter.MinViews {
			return false
		}
	}
	if filter.MinPageCount != nil {
		pageCount, ok := recordInt(r.fields["page_count"])
		if !ok || pageCount < *filter.MinPageCount {
			return false
		}
	}
	return true
}

// MarshalJSON 保留记录中的未知字段，并始终输出一个 JSON 对象。
func (r Record) MarshalJSON() ([]byte, error) {
	if r.fields == nil {
		return nil, errors.New("record is not initialized")
	}
	return json.Marshal(r.fields)
}

// UnmarshalJSON 让 MCP structured content 等 JSON 消费者可以把 records 重新解码为
// Record，同时复用 NDJSON 输入的完整校验、ID 归一与版本元数据清理规则。
func (r *Record) UnmarshalJSON(data []byte) error {
	parsed, err := ParseRecordJSON(data)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// ParseRecordJSON 解析一行 NDJSON 中的单个对象，并保留未知字段。
func ParseRecordJSON(line []byte) (Record, error) {
	fields, err := decodeRecordObject(line)
	if err != nil {
		return Record{}, err
	}
	rawID, exists := fields["id"]
	id, err := normalizeRecordID(rawID, exists)
	if err != nil {
		return Record{}, err
	}
	fields["id"] = id
	return newRecord(fields)
}

func recordFromDTO(value any, sourceID int64, recordType, url string) (Record, error) {
	if sourceID <= 0 {
		return Record{}, errors.New("record id must be positive")
	}
	body, err := json.Marshal(value)
	if err != nil {
		return Record{}, err
	}
	fields, err := decodeRecordObject(body)
	if err != nil {
		return Record{}, err
	}
	fields["id"] = strconv.FormatInt(sourceID, 10)
	fields["type"] = recordType
	fields["url"] = url
	return newRecord(fields)
}

// decodeRecordObject 对每条记录只解码一次；UseNumber 避免未知字段经过 float64 丢失精度。
func decodeRecordObject(data []byte) (map[string]any, error) {
	if !json.Valid(data) {
		return nil, errors.New("invalid record JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("invalid record JSON object: %w", err)
	}
	fields, ok := value.(map[string]any)
	if !ok || fields == nil {
		return nil, errors.New("invalid record JSON object: record must be a JSON object")
	}
	return fields, nil
}

func newRecord(fields map[string]any) (Record, error) {
	if fields == nil {
		return Record{}, errors.New("record must be a JSON object")
	}
	removeVersionMetadata(fields)
	if _, err := requiredStringField(fields, "id"); err != nil {
		return Record{}, err
	}
	if _, err := requiredStringField(fields, "type"); err != nil {
		return Record{}, err
	}
	if _, err := requiredStringField(fields, "url"); err != nil {
		return Record{}, err
	}
	return Record{fields: fields}, nil
}

func removeVersionMetadata(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if isVersionMetadataKey(key) {
				delete(current, key)
				continue
			}
			removeVersionMetadata(child)
		}
	case []any:
		for _, child := range current {
			removeVersionMetadata(child)
		}
	}
}

func isVersionMetadataKey(key string) bool {
	_, prohibited := prohibitedRecordMetadataKeys[normalizeRecordMetadataKey(key)]
	return prohibited
}

func normalizeRecordMetadataKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "_", "")
	return strings.ReplaceAll(key, "-", "")
}

func normalizeRecordID(raw any, exists bool) (string, error) {
	if !exists {
		return "", errors.New("record id is required")
	}
	if stringID, ok := raw.(string); ok {
		if stringID == "" {
			return "", errors.New("record id must be a non-empty string")
		}
		return stringID, nil
	}
	number, ok := raw.(json.Number)
	if !ok || !positiveIntegerJSON.MatchString(number.String()) {
		return "", errors.New("record id must be a non-empty string or positive integer")
	}
	value, ok := new(big.Int).SetString(number.String(), 10)
	if !ok || value.Sign() <= 0 {
		return "", errors.New("record id must be a non-empty string or positive integer")
	}
	return value.String(), nil
}

func requiredStringField(fields map[string]any, name string) (string, error) {
	raw, ok := fields[name]
	if !ok {
		return "", fmt.Errorf("record %s is required", name)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("record %s must be a string", name)
	}
	if value == "" {
		return "", fmt.Errorf("record %s must be a non-empty string", name)
	}
	return value, nil
}

func recordHasTags(raw any, wanted []string) bool {
	rawTags, ok := raw.([]any)
	if !ok {
		return false
	}
	found := make(map[string]struct{}, len(rawTags))
	for _, rawTag := range rawTags {
		if tag, ok := rawTag.(string); ok {
			if tag != "" {
				found[tag] = struct{}{}
			}
			continue
		}
		objectTag, ok := rawTag.(map[string]any)
		if !ok {
			continue
		}
		if name := stringField(objectTag["name"]); name != "" {
			found[name] = struct{}{}
		}
	}
	for _, tag := range wanted {
		if _, ok := found[tag]; !ok {
			return false
		}
	}
	return true
}

func recordInt(raw any) (int64, bool) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, false
	}
	value, err := number.Int64()
	if err != nil {
		return 0, false
	}
	return value, true
}

func stringField(raw any) string {
	value, _ := raw.(string)
	return value
}
