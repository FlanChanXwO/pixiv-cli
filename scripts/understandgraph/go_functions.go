package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func normalizeGoFunctions(
	root string,
	nodes []graphNode,
	edges []graphEdge,
	sources []goSource,
) ([]graphNode, []graphEdge, map[string]json.RawMessage, error) {
	fingerprintPath := filepath.Join(root, ".understand-anything", "fingerprints.json")
	fingerprints, err := readJSONObject(fingerprintPath)
	if err != nil {
		return nil, nil, nil, err
	}
	fingerprintFiles, err := decodeField[map[string]json.RawMessage](fingerprints, "files")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fingerprints: %w", err)
	}

	sourceByPath := make(map[string]goSource, len(sources))
	for _, source := range sources {
		sourceByPath[source.Path] = source
		if err := normalizeFingerprintFunctions(fingerprintFiles, source); err != nil {
			return nil, nil, nil, err
		}
	}
	if err := setField(fingerprints, "files", fingerprintFiles); err != nil {
		return nil, nil, nil, err
	}

	rewrites := make(map[string]string)
	callCandidates := make(map[string]goCallCandidates)
	for index := range nodes {
		node := &nodes[index]
		source, ok := sourceByPath[node.FilePath]
		if !ok || node.Type != "function" {
			continue
		}
		function, err := functionAtLine(source.Functions, node.LineRange)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("normalize function node %s: %w", node.ID, err)
		}
		oldID := node.ID
		node.Name = function.QualifiedName
		node.ID = "function:" + node.FilePath + ":" + function.QualifiedName
		bareID := "function:" + node.FilePath + ":" + function.Name
		candidates := callCandidates[bareID]
		if function.Receiver == "" {
			candidates.free = append(candidates.free, node.ID)
		} else {
			candidates.methods = append(candidates.methods, node.ID)
		}
		callCandidates[bareID] = candidates
		if previous, exists := rewrites[oldID]; exists && previous != node.ID {
			return nil, nil, nil, fmt.Errorf("function node ID %s maps to multiple declarations", oldID)
		}
		rewrites[oldID] = node.ID
	}

	for index := range edges {
		edge := &edges[index]
		if edge.Type == "calls" {
			if _, _, bare := bareFunctionID(edge.Target); bare {
				candidates := callCandidates[edge.Target]
				switch {
				case len(candidates.free) == 1:
					edge.Target = candidates.free[0]
				case len(candidates.free) > 1:
					return nil, nil, nil, fmt.Errorf("ambiguous Go calls edge target %s has %d free function nodes", edge.Target, len(candidates.free))
				case len(candidates.methods) == 1:
					edge.Target = candidates.methods[0]
				case len(candidates.methods) > 1:
					// 没有 free function node 时，bare target 无法区分多个同名 methods，必须显式失败。
					return nil, nil, nil, fmt.Errorf("ambiguous Go calls edge target %s has %d method nodes", edge.Target, len(candidates.methods))
				}
			}
		}
		if rewritten, ok := rewrites[edge.Source]; ok {
			edge.Source = rewritten
		}
		if rewritten, ok := rewrites[edge.Target]; ok {
			edge.Target = rewritten
		}
	}
	return nodes, edges, fingerprints, nil
}

type goCallCandidates struct {
	free    []string
	methods []string
}

func normalizeFingerprintFunctions(files map[string]json.RawMessage, source goSource) error {
	raw, ok := files[source.Path]
	if !ok {
		return fmt.Errorf("fingerprints are missing Go source %s", source.Path)
	}
	var fingerprint map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fingerprint); err != nil {
		return fmt.Errorf("decode fingerprint %s: %w", source.Path, err)
	}
	var contentHash string
	if err := json.Unmarshal(fingerprint["contentHash"], &contentHash); err != nil {
		return fmt.Errorf("decode fingerprint contentHash %s: %w", source.Path, err)
	}
	if contentHash != source.ContentHash {
		return fmt.Errorf("fingerprint contentHash mismatch for %s: got %s, source has %s", source.Path, contentHash, source.ContentHash)
	}
	var totalLines int
	if err := json.Unmarshal(fingerprint["totalLines"], &totalLines); err != nil {
		return fmt.Errorf("decode fingerprint totalLines %s: %w", source.Path, err)
	}
	if totalLines != source.TotalLines {
		return fmt.Errorf("fingerprint totalLines mismatch for %s: got %d, source has %d", source.Path, totalLines, source.TotalLines)
	}
	var functions []map[string]json.RawMessage
	if rawFunctions, ok := fingerprint["functions"]; ok {
		if err := json.Unmarshal(rawFunctions, &functions); err != nil {
			return fmt.Errorf("decode fingerprint functions %s: %w", source.Path, err)
		}
	}
	if len(functions) != len(source.Functions) {
		return fmt.Errorf("fingerprint function count mismatch for %s: graph=%d source=%d", source.Path, len(functions), len(source.Functions))
	}
	// bundled generator 与 Go AST 都按源码声明顺序输出；逐项核验后才能安全补接收者信息。
	for index, function := range source.Functions {
		var currentName string
		if err := json.Unmarshal(functions[index]["name"], &currentName); err != nil {
			return fmt.Errorf("decode fingerprint function name %s[%d]: %w", source.Path, index, err)
		}
		if currentName != function.Name {
			return fmt.Errorf("fingerprint function order mismatch for %s[%d]: got %q, source has %q", source.Path, index, currentName, function.Name)
		}
		var lineCount int
		if err := json.Unmarshal(functions[index]["lineCount"], &lineCount); err != nil {
			return fmt.Errorf("decode fingerprint function lineCount %s[%d]: %w", source.Path, index, err)
		}
		sourceLineCount := function.EndLine - function.StartLine + 1
		if lineCount != sourceLineCount {
			return fmt.Errorf(
				"fingerprint function lineCount mismatch for %s[%d] %s: got %d, source has %d",
				source.Path, index, function.Name, lineCount, sourceLineCount,
			)
		}
		qualified, err := json.Marshal(function.QualifiedName)
		if err != nil {
			return err
		}
		functions[index]["qualifiedName"] = qualified
		if function.Receiver != "" {
			receiver, err := json.Marshal(function.Receiver)
			if err != nil {
				return err
			}
			functions[index]["receiver"] = receiver
		} else {
			delete(functions[index], "receiver")
		}
	}
	encoded, err := json.Marshal(functions)
	if err != nil {
		return err
	}
	fingerprint["functions"] = encoded
	files[source.Path], err = json.Marshal(fingerprint)
	return err
}

func functionAtLine(functions []goFunction, lineRange []int) (goFunction, error) {
	if len(lineRange) != 2 {
		return goFunction{}, fmt.Errorf("lineRange must contain exactly start and end, got %d values", len(lineRange))
	}
	var matches []goFunction
	for _, function := range functions {
		if function.StartLine == lineRange[0] {
			matches = append(matches, function)
		}
	}
	if len(matches) != 1 {
		return goFunction{}, fmt.Errorf("line %d matches %d declarations", lineRange[0], len(matches))
	}
	function := matches[0]
	if function.EndLine != lineRange[1] {
		return goFunction{}, fmt.Errorf(
			"lineRange [%d,%d] does not match AST [%d,%d]",
			lineRange[0], lineRange[1], function.StartLine, function.EndLine,
		)
	}
	return function, nil
}

func bareFunctionID(id string) (string, string, bool) {
	if !strings.HasPrefix(id, "function:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(id, "function:")
	separator := strings.LastIndex(rest, ":")
	if separator < 0 {
		return "", "", false
	}
	name := rest[separator+1:]
	if strings.Contains(name, ".") {
		return "", "", false
	}
	return rest[:separator], name, true
}
