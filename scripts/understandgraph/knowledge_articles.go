package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// normalizeKnowledgeArticles 用当前 docs 源文件刷新 article 全文，避免 generator 的展示截断进入入库快照。
func normalizeKnowledgeArticles(root string, graph map[string]json.RawMessage) error {
	nodes, err := decodeField[[]graphNode](graph, "nodes")
	if err != nil {
		return fmt.Errorf("docs graph: %w", err)
	}
	docsRoot := filepath.Join(root, "docs")
	resolvedDocsRoot, err := filepath.EvalSymlinks(docsRoot)
	if err != nil {
		return fmt.Errorf("resolve docs root: %w", err)
	}
	resolvedDocsRoot, err = filepath.Abs(resolvedDocsRoot)
	if err != nil {
		return fmt.Errorf("resolve absolute docs root: %w", err)
	}
	for index := range nodes {
		node := &nodes[index]
		if node.Type != "article" {
			continue
		}
		if node.FilePath == "" {
			return fmt.Errorf("docs graph article %s is missing filePath", node.ID)
		}
		relativePath := filepath.Clean(filepath.FromSlash(node.FilePath))
		// article path 来自生成产物，但仍需禁止越出 docs 根目录读取任意本机文件。
		if filepath.IsAbs(relativePath) || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			return fmt.Errorf("docs graph article %s has filePath outside docs: %s", node.ID, node.FilePath)
		}
		var knowledgeMeta map[string]json.RawMessage
		if len(node.KnowledgeMeta) == 0 {
			return fmt.Errorf("docs graph article %s is missing knowledgeMeta", node.ID)
		}
		if err := json.Unmarshal(node.KnowledgeMeta, &knowledgeMeta); err != nil {
			return fmt.Errorf("docs graph article %s knowledgeMeta must be an object: %w", node.ID, err)
		}
		if knowledgeMeta == nil {
			return fmt.Errorf("docs graph article %s knowledgeMeta must be an object", node.ID)
		}
		sourcePath := filepath.Join(docsRoot, relativePath)
		resolvedSourcePath, err := filepath.EvalSymlinks(sourcePath)
		if err != nil {
			return fmt.Errorf("read docs graph article %s source %s: resolve path: %w", node.ID, node.FilePath, err)
		}
		resolvedSourcePath, err = filepath.Abs(resolvedSourcePath)
		if err != nil {
			return fmt.Errorf("resolve absolute docs graph article %s source %s: %w", node.ID, node.FilePath, err)
		}
		resolvedRelativePath, err := filepath.Rel(resolvedDocsRoot, resolvedSourcePath)
		if err != nil {
			return fmt.Errorf("resolve docs graph article %s source %s relative to docs: %w", node.ID, node.FilePath, err)
		}
		if resolvedRelativePath == ".." || strings.HasPrefix(resolvedRelativePath, ".."+string(filepath.Separator)) {
			return fmt.Errorf("docs graph article %s source %s resolves outside docs", node.ID, node.FilePath)
		}
		content, err := os.ReadFile(resolvedSourcePath)
		if err != nil {
			return fmt.Errorf("read docs graph article %s source %s: %w", node.ID, node.FilePath, err)
		}
		if !utf8.Valid(content) {
			return fmt.Errorf("docs graph article %s source %s is not valid UTF-8", node.ID, node.FilePath)
		}
		digest := sha256.Sum256(content)
		if err := setRawField(knowledgeMeta, "content", string(content)); err != nil {
			return fmt.Errorf("encode docs graph article %s content: %w", node.ID, err)
		}
		if err := setRawField(knowledgeMeta, "contentHash", fmt.Sprintf("%x", digest)); err != nil {
			return fmt.Errorf("encode docs graph article %s contentHash: %w", node.ID, err)
		}
		encoded, err := json.Marshal(knowledgeMeta)
		if err != nil {
			return fmt.Errorf("encode docs graph article %s knowledgeMeta: %w", node.ID, err)
		}
		node.KnowledgeMeta = encoded
		node.rawFields["knowledgeMeta"] = encoded
	}
	if err := setField(graph, "nodes", nodes); err != nil {
		return fmt.Errorf("docs graph: %w", err)
	}
	return nil
}
