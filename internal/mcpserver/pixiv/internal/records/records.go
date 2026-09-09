// Package records 持有 Pixiv MCP 各 tool 共享的 record 构造、输出 schema 与
// 本地筛选。它从 public DTO 构造 record.Record，保持未知字段与精确数字；
// CLI 与 MCP 共用同一 Record 机制。
package records

import (
	"fmt"

	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Result 统一实体查询的 MCP 文本摘要。完整实体只存在于 structured records，
// Content 绝不复制 JSON，以便客户端始终只消费一个机器可读载荷。
func Result(records []record.Record, isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = fmt.Sprintf("Retrieved %d records.", len(records))
	}
	return &mcp.CallToolResult{
		IsError: isError,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

// ErrorMessage 返回统一错误摘要。
func ErrorMessage(err error) string {
	return "Error: " + err.Error()
}

// FromArtworks 构造 artwork records。
func FromArtworks(items []pixiv.Artwork) ([]record.Record, error) {
	records := make([]record.Record, 0, len(items))
	for _, item := range items {
		record, err := FromArtwork(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// FromNovels 构造 novel records。
func FromNovels(items []pixiv.Novel) ([]record.Record, error) {
	records := make([]record.Record, 0, len(items))
	for _, item := range items {
		record, err := FromNovel(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// FromUserPreviews 构造 user records。
func FromUserPreviews(items []pixiv.UserPreview) ([]record.Record, error) {
	records := make([]record.Record, 0, len(items))
	for _, item := range items {
		record, err := FromUserPreview(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// FromArtwork 把单个 artwork DTO 转为 record。
func FromArtwork(artwork pixiv.Artwork) (record.Record, error) {
	return record.RecordFromArtworkDTO(pixiv.ToArtworkDTO(artwork))
}

// FromNovel 把单个 novel DTO 转为 record。
func FromNovel(novel pixiv.Novel) (record.Record, error) {
	return record.RecordFromNovelDTO(pixiv.ToNovelDTO(novel))
}

// FromUserPreview 把单个 user preview DTO 转为 record。
func FromUserPreview(preview pixiv.UserPreview) (record.Record, error) {
	return record.RecordFromUserPreviewDTO(pixiv.ToUserPreviewDTO(preview))
}

// FromUserDetail 把 user detail DTO 转为 record。
func FromUserDetail(detail pixiv.UserDetail) (record.Record, error) {
	return record.RecordFromUserDetailDTO(pixiv.ToUserDetailDTO(detail))
}

// OpenObjectSchema 返回允许任意附加属性的开放对象 schema。
func OpenObjectSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{}}
}

// PaginationOutputSchema 返回分页输出的开放 schema。
func PaginationOutputSchema() *jsonschema.Schema {
	return OpenObjectSchema()
}

// CommentOutputSchema 返回评论输出的 schema。
func CommentOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"comments": {
				Type:  "array",
				Items: OpenObjectSchema(),
			},
			"pagination":     PaginationOutputSchema(),
			"total":          {Type: "integer"},
			"access_control": OpenObjectSchema(),
		},
		Required:             []string{"comments", "pagination"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// NovelContentOutputSchema 返回小说正文输出的 schema。
func NovelContentOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"content": OpenObjectSchema(),
		},
		Required:             []string{"content"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// UserDetailOutputSchema 描述只返回实体记录的 detail envelope。
func UserDetailOutputSchema() *jsonschema.Schema {
	return singleRecordsOutputSchema()
}

// NovelDetailOutputSchema 描述小说详情的结构化 envelope。
func NovelDetailOutputSchema() *jsonschema.Schema {
	return singleRecordsOutputSchema()
}

// NovelSeriesOutputSchema 描述小说系列元数据与实体记录的 envelope。
func NovelSeriesOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"series": {
				Type:                 "object",
				AdditionalProperties: &jsonschema.Schema{},
			},
			"records": recordArraySchema(),
			"pagination": {
				Type:                 "object",
				AdditionalProperties: &jsonschema.Schema{},
			},
		},
		Required:             []string{"series", "records", "pagination"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// BookmarkTagsOutputSchema 返回收藏标签输出的 schema。
func BookmarkTagsOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"bookmark_tags": {
				Type:  "array",
				Items: OpenObjectSchema(),
			},
			"pagination": PaginationOutputSchema(),
		},
		Required:             []string{"bookmark_tags", "pagination"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// BookmarkDetailOutputSchema 返回收藏详情输出的 schema。
func BookmarkDetailOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"bookmarked": {Type: "boolean"},
			"restrict":   {Type: "string"},
			"tags": {
				Type:  "array",
				Items: &jsonschema.Schema{Type: "string"},
			},
		},
		Required:             []string{"bookmarked", "tags"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// RecordsOutputSchema 描述共享实体 records 协议。Record 保留 SDK 和调用方的
// 未知字段，不能由 Go 的未导出字段自动推导为封闭对象；因此显式允许记录对象的
// 额外属性，同时约束每条记录都具备稳定身份字段。
func RecordsOutputSchema() *jsonschema.Schema {
	allowAdditionalProperties := &jsonschema.Schema{}
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"records": recordArraySchema(),
			"pagination": {
				Type:                 "object",
				AdditionalProperties: allowAdditionalProperties,
			},
			"filter": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"min":          {Type: "integer"},
					"max":          {Type: "integer"},
					"membership":   {Type: "string"},
					"strategy":     {Type: "string"},
					"completeness": {Type: "string"},
				},
				AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			},
			"series": {
				Type:                 "object",
				AdditionalProperties: allowAdditionalProperties,
			},
			"comments": {
				Type:  "array",
				Items: &jsonschema.Schema{Type: "object", AdditionalProperties: allowAdditionalProperties},
			},
			"total": {
				Type: "integer",
			},
			"access_control": {
				Type:                 "object",
				AdditionalProperties: allowAdditionalProperties,
			},
			"bookmark_tags": {
				Type:  "array",
				Items: &jsonschema.Schema{Type: "object", AdditionalProperties: allowAdditionalProperties},
			},
			"bookmarked": {
				Type: "boolean",
			},
			"restrict": {
				Type: "string",
			},
			"tags": {
				Type:  "array",
				Items: &jsonschema.Schema{Type: "string"},
			},
			"content": {
				Type:                 "object",
				AdditionalProperties: allowAdditionalProperties,
			},
		},
		Required:             []string{"records"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// RecommendedOutputSchema 描述 recommended 的四路聚合 envelope。推荐流的
// pagination 只允许已公开的四个 subtype，SDK continuation 仍留在适配层。
func RecommendedOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"records": recordArraySchema(),
			"pagination": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"illust": PaginationOutputSchema(),
					"manga":  PaginationOutputSchema(),
					"novel":  PaginationOutputSchema(),
					"user":   PaginationOutputSchema(),
				},
				AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			},
		},
		Required:             []string{"records", "pagination"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// TrendingTagsOutputSchema 描述 trending_tags_illust 的稳定顶层 envelope；
// tags.items 直接由 public DTO 推导，确保嵌套 artwork 的可选字段与实际 JSON
// 序列化契约同步，同时不把 SDK continuation 或原始响应字段提升为顶层协议。
func TrendingTagsOutputSchema() *jsonschema.Schema {
	tagSchema, err := jsonschema.For[pixiv.TrendingTagDTO](nil)
	if err != nil {
		// TrendingTagDTO 只包含 JSON Schema 支持的 public DTO 字段；如果这里
		// 失败，说明 DTO 契约本身已超出 schema 支持范围，继续注册错误 schema
		// 会比在 composition root 直接暴露问题更危险。
		panic(err)
	}
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"tags": {
				Type:  "array",
				Items: tagSchema,
			},
			"text": {Type: "string"},
		},
		Required:             []string{"tags", "text"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func singleRecordsOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           map[string]*jsonschema.Schema{"records": recordArraySchema()},
		Required:             []string{"records"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func recordArraySchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:  "array",
		Items: recordSchema(),
	}
}

func recordSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:     "object",
		Required: []string{"id", "type", "url"},
		Properties: map[string]*jsonschema.Schema{
			"id":   {Type: "string"},
			"type": {Type: "string"},
			"url":  {Type: "string"},
		},
		AdditionalProperties: &jsonschema.Schema{},
	}
}

// ResourceOut 是资源引用的安全输出形态。
type ResourceOut struct {
	Ref                 string `json:"ref"`
	RequiresCredentials bool   `json:"requires_credentials,omitempty"`
}

// ResourceOutFrom 把 opaque Resource 转为安全输出。
func ResourceOutFrom(res sdk.Resource) *ResourceOut {
	dto := sdk.ToResourceDTO(res)
	if dto == nil {
		return nil
	}
	return &ResourceOut{Ref: dto.Ref, RequiresCredentials: dto.RequiresCredentials}
}
