package record

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var positiveIntegerJSON = regexp.MustCompile(`^[1-9][0-9]*$`)

// prohibitedRecordMetadataKeys 是记录协议明确禁止输出的版本/模式元数据键。
// 键会忽略大小写、下划线和连字符，以同时覆盖 api_version 与 apiVersion。
// 这里必须保持白名单，不能按包含 "version" 判断，避免误删 conversion 等业务字段。
var prohibitedRecordMetadataKeys = map[string]struct{}{
	"version": {}, "schema": {}, "apiversion": {}, "schemaversion": {}, "protocolversion": {},
	"formatversion": {}, "recordversion": {}, "sdkversion": {}, "mcpversion": {}, "cliversion": {}, "versioninfo": {},
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
