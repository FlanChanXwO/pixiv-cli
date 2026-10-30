package schemas

import (
	"reflect"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/google/jsonschema-go/jsonschema"
)

// OpenObjectSchema 返回允许任意附加属性的对象 schema，让 SDK 记录的未知字段通过。
func OpenObjectSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{}}
}

// CommentInputSchema 返回作品与小说评论共用的封闭输入 schema；stamp_id 只属于评论
// mutation，读接口不接受。
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

// CommentOutputSchema 返回评论输出的 schema，comments 与 pagination 必填。
func CommentOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"comments": {
				Type:  "array",
				Items: OpenObjectSchema(),
			},
			"pagination":     OpenObjectSchema(),
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

// NovelSeriesOutputSchema 返回小说系列元数据与实体记录 envelope 的 schema。
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
			"pagination": OpenObjectSchema(),
		},
		Required:             []string{"bookmark_tags", "pagination"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// BookmarkTagsAllOutputSchema 返回 bookmark_tags_all 的 typed 标签 envelope schema；
// bookmark_tags 仍沿用开放的对象 schema。
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
						"content_type": {Type: "string", Enum: []any{string(pixiv.ReferenceKindArtwork), string(pixiv.ReferenceKindNovel)}},
					},
					Required:             []string{"name", "count", "content_type"},
					AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
				},
			},
			"pagination": OpenObjectSchema(),
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

// RecordsOutputSchema 返回共享 records envelope 的 schema。记录需要保留 SDK 与调用方
// 的未知字段，所以记录对象开放；每条记录仍须具备稳定的身份字段。
func RecordsOutputSchema() *jsonschema.Schema {
	// jsonschema resolver 要求树形节点，开放属性 schema 的每个位置各用一个新指针。
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

// RecommendedOutputSchema 按实体分区公开 records 与 pagination。
func RecommendedOutputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"artworks": RecordsOutputSchema(), "novels": RecordsOutputSchema(), "users": RecordsOutputSchema(),
		},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// TrendingTagsOutputSchema 返回 trending_tags_illust 顶层 envelope 的 schema。
// tags.items 从公开模型推导，嵌套作品的可选字段与 JSON 序列化保持一致；SDK
// continuation 与原始响应字段不提升为顶层协议。
func TrendingTagsOutputSchema() *jsonschema.Schema {
	// Resource 由 MarshalJSON 决定 JSON 形态，只输出 opaque ref 与 requires_credentials，
	// 零值序列化成 null。schema 推导看不到方法实现，所以这里显式声明。
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
		// TrendingTag 只含 JSON Schema 支持的公开字段；推导失败说明模型已超出 schema
		// 能力，此时宁可启动失败，也不要注册一个错误的 schema。
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

// DetailOutputSchema 返回只含实体记录数组的 detail envelope schema。
func DetailOutputSchema() *jsonschema.Schema {
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
