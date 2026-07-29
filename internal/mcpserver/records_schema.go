package mcpserver

import "github.com/google/jsonschema-go/jsonschema"

// recordsOutputSchema 描述共享实体 records 协议。Record 保留 SDK 和调用方的
// 未知字段，不能由 Go 的未导出字段自动推导为封闭对象；因此显式允许记录对象的
// 额外属性，同时约束每条记录都具备稳定身份字段。
func recordsOutputSchema() *jsonschema.Schema {
	allowAdditionalProperties := &jsonschema.Schema{}
	record := &jsonschema.Schema{
		Type:     "object",
		Required: []string{"id", "type", "url"},
		Properties: map[string]*jsonschema.Schema{
			"id":   {Type: "string"},
			"type": {Type: "string"},
			"url":  {Type: "string"},
		},
		AdditionalProperties: allowAdditionalProperties,
	}
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"records": {
				Type:  "array",
				Items: record,
			},
			"pagination": {
				Type:                 "object",
				AdditionalProperties: allowAdditionalProperties,
			},
		},
		Required:             []string{"records"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}
