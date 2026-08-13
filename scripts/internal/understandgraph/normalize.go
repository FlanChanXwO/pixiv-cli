package understandgraph

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
)

// Normalize 稳定 generator 的 Go package/module 图语义，并同步文档 article 的完整源正文。
func Normalize(root string) error {
	return normalizeWithContainedFileReader(root, readContainedRegularFile)
}

func normalizeWithContainedFileReader(root string, readFile containedFileReader) error {
	ua := filepath.Join(root, ".understand-anything")
	scanPath := filepath.Join(ua, "intermediate", "scan-result.json")
	graphPath := filepath.Join(ua, "knowledge-graph.json")
	docsGraphPath := filepath.Join(root, "docs", ".understand-anything", "knowledge-graph.json")

	scan, err := readJSONObject(scanPath)
	if err != nil {
		return err
	}
	graph, err := readJSONObject(graphPath)
	if err != nil {
		return err
	}
	docsGraph, err := readJSONObject(docsGraphPath)
	if err != nil {
		return err
	}
	files, err := decodeField[[]scanFile](scan, "files")
	if err != nil {
		return fmt.Errorf("scan result: %w", err)
	}
	importMap, err := decodeField[map[string][]string](scan, "importMap")
	if err != nil {
		return fmt.Errorf("scan result: %w", err)
	}
	modulePath, err := readModulePath(root)
	if err != nil {
		return err
	}
	goSources, packages, err := analyzeGoPackages(root, modulePath, files, readFile)
	if err != nil {
		return err
	}

	goPaths := make(map[string]struct{}, len(goSources))
	packageImports := make(map[string][]string, len(goSources))
	for _, source := range goSources {
		goPaths[source.Path] = struct{}{}
		importMap[source.Path] = []string{}
		packageImports[source.Path] = source.Imports
	}
	packageFiles := make(map[string][]string, len(packages))
	for _, pkg := range packages {
		packageFiles[pkg.ID] = pkg.Files
	}

	nodes, err := decodeField[[]graphNode](graph, "nodes")
	if err != nil {
		return fmt.Errorf("graph: %w", err)
	}
	if err := validateUniqueInputNodeIDs(nodes); err != nil {
		return err
	}
	edges, err := decodeField[[]graphEdge](graph, "edges")
	if err != nil {
		return fmt.Errorf("graph: %w", err)
	}
	nodes, edges, fingerprints, err := normalizeGoFunctions(root, nodes, edges, goSources)
	if err != nil {
		return err
	}
	fileNodeByPath := make(map[string]string, len(goSources))
	goFileNodeIDs := make(map[string]struct{}, len(goSources))
	for _, node := range nodes {
		if _, ok := goPaths[node.FilePath]; !ok || node.Type != "file" {
			continue
		}
		if fileNodeByPath[node.FilePath] != "" {
			return fmt.Errorf("graph contains multiple file nodes for %s", node.FilePath)
		}
		fileNodeByPath[node.FilePath] = node.ID
		goFileNodeIDs[node.ID] = struct{}{}
	}
	for path := range goPaths {
		if fileNodeByPath[path] == "" {
			return fmt.Errorf("graph is missing Go file node for %s", path)
		}
	}

	ownedModuleIDs := make(map[string]struct{})
	for _, node := range nodes {
		if node.Type == "module" && containsString(node.Tags, "go-package") {
			ownedModuleIDs[node.ID] = struct{}{}
		}
	}
	filteredNodes := make([]graphNode, 0, len(nodes)+len(packages))
	seenNodeIDs := make(map[string]struct{}, len(nodes)+len(packages))
	for _, node := range nodes {
		// go-package tag 是本归一化器的 ownership 标记；旧包改名或删除后也必须清走。
		if _, owned := ownedModuleIDs[node.ID]; owned {
			continue
		}
		if _, seen := seenNodeIDs[node.ID]; seen {
			return fmt.Errorf("graph contains duplicate node ID %s", node.ID)
		}
		seenNodeIDs[node.ID] = struct{}{}
		filteredNodes = append(filteredNodes, node)
	}
	nodes = filteredNodes

	filteredEdges := edges[:0]
	for _, edge := range edges {
		_, goSource := goFileNodeIDs[edge.Source]
		_, ownedSource := ownedModuleIDs[edge.Source]
		_, ownedTarget := ownedModuleIDs[edge.Target]
		if ownedSource || ownedTarget {
			continue
		}
		if edge.Type == "imports" && goSource {
			// Go imports 由 AST 重建；这同时移除 generator 的 file fan-out 和上一遍 module 边。
			continue
		}
		filteredEdges = append(filteredEdges, edge)
	}
	edges = filteredEdges
	for _, pkg := range packages {
		if _, exists := seenNodeIDs[pkg.ID]; exists {
			return fmt.Errorf("Go package module ID %s conflicts with non-owned graph node", pkg.ID)
		}
		nodes = append(nodes, packageNode(pkg))
		seenNodeIDs[pkg.ID] = struct{}{}
		for _, member := range pkg.Files {
			edges = append(edges, graphEdge{Source: pkg.ID, Target: fileNodeByPath[member], Type: "contains", Direction: "forward", Weight: 1})
		}
	}
	for _, source := range goSources {
		for _, target := range source.Imports {
			edges = append(edges, graphEdge{Source: fileNodeByPath[source.Path], Target: target, Type: "imports", Direction: "forward", Weight: 0.7})
		}
	}
	edges, err = deduplicateEdges(edges)
	if err != nil {
		return err
	}
	if err := validateGraphEdges(edges, seenNodeIDs); err != nil {
		return err
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(edges, func(i, j int) bool {
		left := edges[i].Source + "\x00" + edges[i].Target + "\x00" + edges[i].Type
		right := edges[j].Source + "\x00" + edges[j].Target + "\x00" + edges[j].Type
		return left < right
	})
	if err := setField(scan, "importMap", importMap); err != nil {
		return err
	}
	if err := setField(scan, "goPackageImportMap", packageImports); err != nil {
		return err
	}
	if err := setField(scan, "goPackageFiles", packageFiles); err != nil {
		return err
	}
	if err := setField(graph, "nodes", nodes); err != nil {
		return err
	}
	if err := setField(graph, "edges", edges); err != nil {
		return err
	}
	if err := normalizeKnowledgeArticles(root, docsGraph, readFile); err != nil {
		return err
	}

	return writeJSONObjects(
		jsonObjectFile{Path: scanPath, Object: scan},
		jsonObjectFile{Path: graphPath, Object: graph},
		jsonObjectFile{Path: filepath.Join(ua, "fingerprints.json"), Object: fingerprints},
		jsonObjectFile{Path: docsGraphPath, Object: docsGraph},
	)
}

func validateUniqueInputNodeIDs(nodes []graphNode) error {
	seen := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if _, exists := seen[node.ID]; exists {
			return fmt.Errorf("input graph contains duplicate node ID %s", node.ID)
		}
		seen[node.ID] = struct{}{}
	}
	return nil
}

func validateGraphEdges(edges []graphEdge, nodeIDs map[string]struct{}) error {
	for _, edge := range edges {
		if _, exists := nodeIDs[edge.Source]; !exists {
			return fmt.Errorf("graph edge %s -> %s (%s) has missing source node", edge.Source, edge.Target, edge.Type)
		}
		if _, exists := nodeIDs[edge.Target]; !exists {
			return fmt.Errorf("graph edge %s -> %s (%s) has missing target node", edge.Source, edge.Target, edge.Type)
		}
	}
	return nil
}

func packageNode(pkg goPackage) graphNode {
	return graphNode{
		ID: pkg.ID, Type: "module", Name: pkg.Name,
		Summary: "Go package " + pkg.Name + " 的稳定模块节点，聚合该 package clause 对应的源文件。",
		Tags:    []string{"go-package", "module"}, Complexity: "simple",
	}
}

func deduplicateEdges(edges []graphEdge) ([]graphEdge, error) {
	seen := make(map[string]string, len(edges))
	result := make([]graphEdge, 0, len(edges))
	for _, edge := range edges {
		key := edge.Source + "\x00" + edge.Target + "\x00" + edge.Type
		encoded, err := json.Marshal(edge)
		if err != nil {
			return nil, fmt.Errorf("encode graph edge %s -> %s (%s) for deduplication: %w", edge.Source, edge.Target, edge.Type, err)
		}
		if previous, exists := seen[key]; exists {
			if previous != string(encoded) {
				return nil, fmt.Errorf("conflicting duplicate graph edge %s -> %s (%s)", edge.Source, edge.Target, edge.Type)
			}
			continue
		}
		seen[key] = string(encoded)
		result = append(result, edge)
	}
	return result, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
