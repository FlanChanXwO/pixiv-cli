// MCP output schemas preserve open entity records and explicit wire envelopes.

package schemas

import (
	"reflect"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/google/jsonschema-go/jsonschema"
)

// OpenObjectSchema 返回允许任意附加属性的开放对象 schema。
func OpenObjectSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{}}
}

// PaginationOutputSchema 返回分页输出的开放 schema。
func PaginationOutputSchema() *jsonschema.Schema {
	return OpenObjectSchema()
}

// CommentInputSchema 返回 artwork/novel comments 共用的稳定输入 schema。
// stamp_id 属于评论 mutation 的独立字段，不能借 read tool 的兼容边界暴露。
func CommentInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id"},
		"properties": map[string]any{
			"id":    map[string]any{"type": "integer", "minimum": 1, "description": "Positive artwork or novel ID."},
			"page":  map[string]any{"type": "integer", "minimum": 1, "description": "1-based logical page; requires a positive limit."},
			"limit": map[string]any{"type": "integer", "minimum": 0, "description": "Maximum comments; 0 returns all; omitted reads one upstream batch."},
		},
	}
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

// BookmarkTagsAllOutputSchema 描述 additive bookmark_tags_all 的 typed 标签
// envelope。legacy bookmark_tags 继续使用开放的旧标签对象 schema。
func BookmarkTagsAllOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"bookmark_tags": {
				Type: "array",
				Items: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name":         {Type: "string"},
						"count":        {Type: "integer"},
						"content_type": {Type: "string", Enum: []any{"artwork", "novel"}},
					},
					Required:             []string{"name", "count", "content_type"},
					AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
				},
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
	// jsonschema resolver 要求树形节点；开放属性 schema 也不能跨位置共享指针。
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"records": recordArraySchema(),
			"pagination": {
				Type:                 "object",
				AdditionalProperties: &jsonschema.Schema{},
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
				AdditionalProperties: &jsonschema.Schema{},
			},
			"comments": {
				Type:  "array",
				Items: &jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{}},
			},
			"total": {
				Type: "integer",
			},
			"access_control": {
				Type:                 "object",
				AdditionalProperties: &jsonschema.Schema{},
			},
			"bookmark_tags": {
				Type:  "array",
				Items: &jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{}},
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
				AdditionalProperties: &jsonschema.Schema{},
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
// tags.items 直接由 public 模型推导，确保嵌套 artwork 的可选字段与实际 JSON
// 序列化契约同步，同时不把 SDK continuation 或原始响应字段提升为顶层协议。
func TrendingTagsOutputSchema() *jsonschema.Schema {
	// Resource 的 JSON 形态由它的 MarshalJSON 决定：只有 opaque ref 与
	// requires_credentials，序列化零值时是 null。schema 推导看不到方法实现，
	// 因此显式给出这一类型，保持与输出契约一致。
	tagSchema, err := jsonschema.For[pixiv.TrendingTag](&jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			reflect.TypeFor[sdk.Resource](): {
				Types:                []string{"null", "object"},
				Properties:           map[string]*jsonschema.Schema{"ref": {Type: "string"}, "requires_credentials": {Type: "boolean"}},
				Required:             []string{"ref"},
				AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			},
		},
	})
	if err != nil {
		// TrendingTag 只包含 JSON Schema 支持的公开模型字段；如果这里失败，
		// 说明契约本身已超出 schema 支持范围，继续注册错误 schema 会比在
		// composition root 直接暴露问题更危险。
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
