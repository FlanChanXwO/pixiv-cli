// Package workflowpolicy 提供 release 与 native evidence verifier 共用的 YAML AST 安全策略。
package workflowpolicy

import (
	"errors"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// secretReferencePattern 识别 GitHub expression 内独立的 secrets context，
// 包括 bare、toJSON(secrets)、dot 和 bracket 访问形式。
var secretReferencePattern = regexp.MustCompile(`(?is)\$\{\{[^}]*\bsecrets\b[^}]*\}\}`)

// RejectAmbiguousYAML 在 verifier 读取字段前拒绝有歧义的 YAML 构造。
func RejectAmbiguousYAML(node *yaml.Node) error {
	if node == nil {
		return errors.New("workflow must not contain nil YAML nodes")
	}
	if node.Kind == yaml.AliasNode {
		return errors.New("workflow must not use YAML aliases")
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%2 != 0 {
			return errors.New("workflow mappings must contain key-value pairs")
		}
		keys := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.Kind != yaml.ScalarNode {
				return errors.New("workflow mapping keys must be scalars")
			}
			if key.Value == "<<" {
				return errors.New("workflow must not use YAML merge keys")
			}
			if _, duplicate := keys[key.Value]; duplicate {
				return fmt.Errorf("workflow must not contain duplicate mapping key %q", key.Value)
			}
			keys[key.Value] = struct{}{}
			if err := RejectAmbiguousYAML(key); err != nil {
				return err
			}
			if err := RejectAmbiguousYAML(value); err != nil {
				return err
			}
		}
		return nil
	}
	for _, child := range node.Content {
		if err := RejectAmbiguousYAML(child); err != nil {
			return err
		}
	}
	return nil
}

// MappingValue 返回 mapping 中指定 key 的值；调用方须先通过 RejectAmbiguousYAML
// 排除重复键，确保返回首个值不会掩盖歧义。
func MappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1], true
		}
	}
	return nil, false
}

// HasExactMappingKeys 判断 mapping 是否恰好包含给定 allowlist，顺序不影响结果。
func HasExactMappingKeys(mapping *yaml.Node, keys ...string) bool {
	if mapping == nil || mapping.Kind != yaml.MappingNode || len(mapping.Content) != len(keys)*2 {
		return false
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	if len(allowed) != len(keys) {
		return false
	}
	seen := make(map[string]struct{}, len(keys))
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		key := mapping.Content[index]
		if key.Kind != yaml.ScalarNode {
			return false
		}
		if _, ok := allowed[key.Value]; !ok {
			return false
		}
		if _, duplicate := seen[key.Value]; duplicate {
			return false
		}
		seen[key.Value] = struct{}{}
	}
	return len(seen) == len(allowed)
}

// RequireScalar 要求 mapping 的指定字段为精确 scalar 值，并保留 verifier 既有错误文本。
func RequireScalar(mapping *yaml.Node, key, want string) error {
	value, ok := MappingValue(mapping, key)
	if !ok || value.Kind != yaml.ScalarNode || value.Value != want {
		return fmt.Errorf("%s must equal %q", key, want)
	}
	return nil
}

// ContainsSecretReference 递归检查任一 scalar 是否引用 GitHub secrets context。
func ContainsSecretReference(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.ScalarNode && secretReferencePattern.MatchString(node.Value) {
		return true
	}
	for _, child := range node.Content {
		if ContainsSecretReference(child) {
			return true
		}
	}
	return false
}
