// Package workflowpolicy 提供 release 与 native evidence verifier 共用的 YAML AST 安全策略。
package workflowpolicy

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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
	if node.Kind == yaml.ScalarNode && scalarContainsSecretReference(node.Value) {
		return true
	}
	for _, child := range node.Content {
		if ContainsSecretReference(child) {
			return true
		}
	}
	return false
}

func scalarContainsSecretReference(value string) bool {
	for searchFrom := 0; searchFrom < len(value); {
		relativeStart := strings.Index(value[searchFrom:], "${{")
		if relativeStart < 0 {
			return false
		}
		expressionStart := searchFrom + relativeStart + len("${{")
		expressionEnd, containsSecret, closed := scanGitHubExpression(value, expressionStart)
		if !closed {
			return false
		}
		if containsSecret {
			return true
		}
		searchFrom = expressionEnd
	}
	return false
}

// scanGitHubExpression 只把单引号字符串外的 }} 视为表达式结束符；GitHub
// expression 以两个连续单引号转义单引号，因此转义字符不会提前退出字符串。
func scanGitHubExpression(value string, start int) (end int, containsSecret, closed bool) {
	inSingleQuotedString := false
	for index := start; index < len(value); {
		if inSingleQuotedString {
			if value[index] != '\'' {
				index++
				continue
			}
			if index+1 < len(value) && value[index+1] == '\'' {
				index += 2
				continue
			}
			inSingleQuotedString = false
			index++
			continue
		}

		if value[index] == '\'' {
			inSingleQuotedString = true
			index++
			continue
		}
		if index+1 < len(value) && value[index] == '}' && value[index+1] == '}' {
			return index + 2, containsSecret, true
		}
		if isSecretContextTokenAt(value, index) {
			containsSecret = true
			index += len("secrets")
			continue
		}
		index++
	}
	return len(value), false, false
}

func isSecretContextTokenAt(value string, index int) bool {
	const token = "secrets"
	if index+len(token) > len(value) || !strings.EqualFold(value[index:index+len(token)], token) {
		return false
	}
	return (index == 0 || !isASCIIIdentifierByte(value[index-1])) &&
		(index+len(token) == len(value) || !isASCIIIdentifierByte(value[index+len(token)]))
}

func isASCIIIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		value == '_'
}
