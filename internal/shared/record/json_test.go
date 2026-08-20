package record_test

import (
	"encoding/json"
	"strings"
	"testing"

	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecordJSONNormalizesNumericIDAndPreservesUnknownFields(t *testing.T) {
	record, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":9007199254740993,
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/9007199254740993",
		"custom":{"enabled":true,"score":1.25},
		"tags":["tag-a"]
	}`))
	require.NoError(t, err)

	got, err := json.Marshal(record)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"9007199254740993",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/9007199254740993",
		"custom":{"enabled":true,"score":1.25},
		"tags":["tag-a"]
	}`, string(got))
	assert.Equal(t, "9007199254740993", record.ID())
}

func TestRecordUnmarshalJSONUsesPipelineValidationAndNormalization(t *testing.T) {
	var record recordpkg.Record
	err := json.Unmarshal([]byte(`{
		"id":9007199254740993,
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/9007199254740993",
		"version":"must-not-survive",
		"unknown":{"value":true}
	}`), &record)
	require.NoError(t, err)
	assert.Equal(t, "9007199254740993", record.ID())
	encoded, err := json.Marshal(record)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"9007199254740993","type":"illust","url":"https://www.pixiv.net/artworks/9007199254740993","unknown":{"value":true}}`, string(encoded))
}

func TestParseRecordJSONRemovesVersionMetadataRecursively(t *testing.T) {
	parsed, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"1",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/1",
		"version":"top-version",
		"schema":{"name":"old"},
		"api_version":"api-version",
		"apiVersion":"api-version-camel",
		"schema_version":"schema-version",
		"schemaVersion":"schema-version-camel",
		"protocol_version":"protocol-version",
		"format_version":"format-version",
		"record_version":"record-version",
		"sdk_version":"sdk-version",
		"mcp_version":"mcp-version",
		"cli_version":"cli-version",
		"version_info":{"name":"old"},
		"conversion":"must-stay",
		"custom":{"keep":true,"version":"nested-version","schemaVersion":"nested-schema","conversion":"nested-conversion"},
		"items":[
			{"keep":"first","apiVersion":"nested-api","protocol_version":"nested-protocol"},
			{"conversion":"second-conversion","nested":[{"format_version":"nested-format","keep":"deep"}]}
		]
	}`))
	require.NoError(t, err)

	got, err := json.Marshal(parsed)
	require.NoError(t, err)
	var object any
	require.NoError(t, json.Unmarshal(got, &object))
	assertNoVersionMetadata(t, object)
	assert.JSONEq(t, `{
		"id":"1",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/1",
		"conversion":"must-stay",
		"custom":{"keep":true,"conversion":"nested-conversion"},
		"items":[
			{"keep":"first"},
			{"conversion":"second-conversion","nested":[{"keep":"deep"}]}
		]
	}`, string(got))
}

func TestParseRecordJSONPreservesUnknownNumbersWithoutFloatConversion(t *testing.T) {
	parsed, err := recordpkg.ParseRecordJSON([]byte(`{
		"id":"1",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/1",
		"custom_integer":9007199254740993123456789,
		"custom_decimal":0.12345678901234567890123456789,
		"nested":{"integer":9223372036854775807,"decimal":1.0000000000000000001}
	}`))
	require.NoError(t, err)

	got, err := json.Marshal(parsed)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"1",
		"type":"illust",
		"url":"https://www.pixiv.net/artworks/1",
		"custom_integer":9007199254740993123456789,
		"custom_decimal":0.12345678901234567890123456789,
		"nested":{"integer":9223372036854775807,"decimal":1.0000000000000000001}
	}`, string(got))
	assert.Contains(t, string(got), `"custom_integer":9007199254740993123456789`)
	assert.Contains(t, string(got), `"custom_decimal":0.12345678901234567890123456789`)
}

func TestParseRecordJSONRejectsInvalidRequiredFields(t *testing.T) {
	for _, test := range []struct {
		name     string
		line     string
		contains string
	}{
		{name: "invalid JSON", line: `{`, contains: "invalid record JSON object"},
		{name: "array", line: `[]`, contains: "invalid record JSON object"},
		{name: "missing id", line: `{"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id is required"},
		{name: "empty id", line: `{"id":"","type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string"},
		{name: "null id", line: `{"id":null,"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string or positive integer"},
		{name: "float id", line: `{"id":1.5,"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string or positive integer"},
		{name: "negative id", line: `{"id":-1,"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string or positive integer"},
		{name: "zero id", line: `{"id":0,"type":"illust","url":"https://www.pixiv.net/artworks/1"}`, contains: "record id must be a non-empty string or positive integer"},
		{name: "missing type", line: `{"id":"1","url":"https://www.pixiv.net/artworks/1"}`, contains: "record type is required"},
		{name: "wrong type type", line: `{"id":"1","type":1,"url":"https://www.pixiv.net/artworks/1"}`, contains: "record type must be a string"},
		{name: "null type", line: `{"id":"1","type":null,"url":"https://www.pixiv.net/artworks/1"}`, contains: "record type must be a string"},
		{name: "empty url", line: `{"id":"1","type":"illust","url":""}`, contains: "record url must be a non-empty string"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := recordpkg.ParseRecordJSON([]byte(test.line))
			require.Error(t, err)
			assert.ErrorContains(t, err, test.contains)
		})
	}
}

func assertNoVersionMetadata(t *testing.T, value any) {
	t.Helper()
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			assert.NotContains(t, testVersionMetadataKeys, normalizeTestMetadataKey(key), "version metadata key %q must be absent", key)
			assertNoVersionMetadata(t, child)
		}
	case []any:
		for _, child := range current {
			assertNoVersionMetadata(t, child)
		}
	}
}

var testVersionMetadataKeys = map[string]struct{}{
	"version": {}, "schema": {}, "apiversion": {}, "schemaversion": {}, "protocolversion": {},
	"formatversion": {}, "recordversion": {}, "sdkversion": {}, "mcpversion": {}, "cliversion": {}, "versioninfo": {},
}

func normalizeTestMetadataKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "_", "")
	return strings.ReplaceAll(key, "-", "")
}
