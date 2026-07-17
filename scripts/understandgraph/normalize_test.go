package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunNormalizeCommand(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	var stderr bytes.Buffer
	require.NoError(t, run([]string{"normalize", "--root", root}, &stderr))
	require.Empty(t, stderr.String())
	require.ErrorContains(t, run([]string{"normalize"}, &stderr), "--root is required")
	require.ErrorContains(t, run([]string{"unknown"}, &stderr), "unknown command")
}

func TestWriteJSONObjectsStagesAllContentBeforeReplacing(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.json")
	secondPath := filepath.Join(root, "second.json")
	require.NoError(t, os.WriteFile(firstPath, []byte("first-original\n"), 0o600))
	require.NoError(t, os.WriteFile(secondPath, []byte("second-original\n"), 0o600))

	err := writeJSONObjects(
		jsonObjectFile{Path: firstPath, Object: map[string]json.RawMessage{"valid": json.RawMessage(`true`)}},
		jsonObjectFile{Path: secondPath, Object: map[string]json.RawMessage{"invalid": json.RawMessage(`{`)}},
	)
	require.ErrorContains(t, err, "encode "+secondPath)
	require.Equal(t, []byte("first-original\n"), readFile(t, firstPath))
	require.Equal(t, []byte("second-original\n"), readFile(t, secondPath))
}

func TestWriteJSONObjectsCleansStagedFilesAfterReplacementFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	require.NoError(t, os.WriteFile(target, []byte("original\n"), 0o640))
	replacementErr := errors.New("replace failed")
	err := writeJSONObjectsWithReplacer(
		func(source, target string) error {
			require.NoError(t, os.WriteFile(source+".recovery", []byte("recovery"), 0o600))
			return replacementErr
		},
		jsonObjectFile{Path: target, Object: map[string]json.RawMessage{"new": json.RawMessage(`true`)}},
	)
	require.ErrorIs(t, err, replacementErr)
	require.ErrorContains(t, err, "after 0 successful replacements")
	require.Equal(t, []byte("original\n"), readFile(t, target))
	require.Empty(t, stagedJSONArtifacts(t, root))
}

func TestWriteJSONObjectsReplacesExistingTargetAndPreservesMode(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	require.NoError(t, os.WriteFile(target, []byte("original\n"), 0o640))
	before, err := os.Stat(target)
	require.NoError(t, err)

	require.NoError(t, writeJSONObjects(
		jsonObjectFile{Path: target, Object: map[string]json.RawMessage{"new": json.RawMessage(`true`)}},
	))
	require.JSONEq(t, `{"new":true}`, string(readFile(t, target)))
	after, err := os.Stat(target)
	require.NoError(t, err)
	require.Equal(t, before.Mode().Perm(), after.Mode().Perm())
	require.Empty(t, stagedJSONArtifacts(t, root))
}

func TestWriteJSONObjectsPreservesCurrentRecoveryMaterialAndCleansFollowingStages(t *testing.T) {
	root := t.TempDir()
	firstTarget := filepath.Join(root, "first.json")
	secondTarget := filepath.Join(root, "second.json")
	require.NoError(t, os.WriteFile(firstTarget, []byte("first-original\n"), 0o600))
	require.NoError(t, os.WriteFile(secondTarget, []byte("second-original\n"), 0o600))
	var preservedSource string
	replacementErr := preservingReplacementError{err: errors.New("restore failed")}
	err := writeJSONObjectsWithReplacer(
		func(source, target string) error {
			preservedSource = source
			require.NoError(t, os.WriteFile(source+".recovery", []byte("old target"), 0o600))
			return replacementErr
		},
		jsonObjectFile{Path: firstTarget, Object: map[string]json.RawMessage{"first": json.RawMessage(`true`)}},
		jsonObjectFile{Path: secondTarget, Object: map[string]json.RawMessage{"second": json.RawMessage(`true`)}},
	)
	require.ErrorIs(t, err, replacementErr)
	require.FileExists(t, preservedSource)
	require.FileExists(t, preservedSource+".recovery")
	require.ElementsMatch(t, []string{preservedSource, preservedSource + ".recovery"}, stagedJSONArtifacts(t, root))
	require.Equal(t, []byte("second-original\n"), readFile(t, secondTarget))
	require.NoError(t, os.Remove(preservedSource))
	require.NoError(t, os.Remove(preservedSource+".recovery"))
}

type preservingReplacementError struct {
	err error
}

func (err preservingReplacementError) Error() string          { return err.err.Error() }
func (err preservingReplacementError) Unwrap() error          { return err.err }
func (preservingReplacementError) PreserveReplacementSource() {}

func TestNormalizeReplacesExpandedGoImportsWithPackageModules(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod":               "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go":               "package a\n",
		"a/a_more.go":          "package a\n",
		"a/a_test.go":          "package a\n",
		"a/external_test.go":   "package a_test\n\nimport _ \"example.com/pixiv/a\"\n",
		"consumer/consumer.go": "package consumer\n\nimport _ \"example.com/pixiv/a\"\n",
	})

	require.NoError(t, Normalize(root))

	var scan map[string]any
	readJSONFile(t, filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json"), &scan)
	imports := stringSliceMap(t, scan["importMap"])
	require.Empty(t, imports["consumer/consumer.go"])
	require.Empty(t, imports["a/external_test.go"])

	packageImports := stringSliceMap(t, scan["goPackageImportMap"])
	require.Equal(t, []string{"module:example.com/pixiv/a"}, packageImports["consumer/consumer.go"])
	require.Equal(t, []string{"module:example.com/pixiv/a"}, packageImports["a/external_test.go"])

	packageFiles := stringSliceMap(t, scan["goPackageFiles"])
	require.Equal(t, []string{"a/a.go", "a/a_more.go", "a/a_test.go"}, packageFiles["module:example.com/pixiv/a"])
	require.Equal(t, []string{"a/external_test.go"}, packageFiles["module:example.com/pixiv/a#a_test"])

	var graph graphFixture
	readJSONFile(t, filepath.Join(root, ".understand-anything", "knowledge-graph.json"), &graph)
	require.Contains(t, graph.NodeIDs(), "module:example.com/pixiv/a")
	require.Contains(t, graph.NodeIDs(), "module:example.com/pixiv/a#a_test")
	require.Contains(t, graph.EdgeKeys(), "file:consumer/consumer.go\x00module:example.com/pixiv/a\x00imports")
	require.Contains(t, graph.EdgeKeys(), "file:a/external_test.go\x00module:example.com/pixiv/a\x00imports")
	for _, edge := range graph.Edges {
		if edge.Type == "imports" {
			require.NotContains(t, edge.Target, "_test.go")
			require.NotContains(t, edge.Target, "file:a/")
		}
	}
}

func TestNormalizeSupportsProductionPackageNameEndingInTest(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod":               "module example.com/pixiv\n\ngo 1.26.3\n",
		"c/c.go":               "package c_test\n",
		"c/c_internal_test.go": "package c_test\n",
		"c/c_external_test.go": "package c_test_test\n",
	})
	require.NoError(t, Normalize(root))
	var scan map[string]any
	readJSONFile(t, filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json"), &scan)
	packageFiles := stringSliceMap(t, scan["goPackageFiles"])
	require.Equal(t, []string{"c/c.go", "c/c_internal_test.go"}, packageFiles["module:example.com/pixiv/c"])
	require.Equal(t, []string{"c/c_external_test.go"}, packageFiles["module:example.com/pixiv/c#c_test_test"])
}

func TestNormalizeRejectsExternalTestPackageInNonTestFile(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod":    "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go":    "package a\n",
		"a/fake.go": "package a_test\n",
	})
	require.ErrorContains(t, Normalize(root), `Go directory "a" contains multiple production packages`)
}

func TestNormalizeClassifiesTestOnlyPackagesLikeGoTool(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod":                  "module example.com/pixiv\n\ngo 1.26.3\n",
		"only/a_internal_test.go": "package only\n",
		"only/b_external_test.go": "package only_test\n",
		"command/main_test.go":    "package main_test\n",
	})
	require.NoError(t, Normalize(root))
	var scan map[string]any
	readJSONFile(t, filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json"), &scan)
	packageFiles := stringSliceMap(t, scan["goPackageFiles"])
	require.Equal(t, []string{"only/a_internal_test.go"}, packageFiles["module:example.com/pixiv/only"])
	require.Equal(t, []string{"only/b_external_test.go"}, packageFiles["module:example.com/pixiv/only#only_test"])
	require.Equal(t, []string{"command/main_test.go"}, packageFiles["module:example.com/pixiv/command#main_test"])
}

func TestNormalizeRejectsUnrelatedPackageInTestFile(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod":          "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go":          "package a\n",
		"a/weird_test.go": "package unrelated\n",
	})
	require.ErrorContains(t, Normalize(root), "Go test source a/weird_test.go has package unrelated; want a or a_test")
}

func TestNormalizeQualifiesEveryGoMethodAndFingerprintsReceiver(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/methods.go": `package a

type Alpha struct{}
type Beta struct{}

func (Alpha) Ping() {}
func (*Beta) Ping() {}
func (Alpha) Solo() {}
func Free() {}
`,
	})

	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["nodes"] = append(graph["nodes"].([]any),
		map[string]any{"id": "function:a/methods.go:Alpha.Ping", "type": "function", "name": "Alpha.Ping", "filePath": "a/methods.go", "lineRange": []int{6, 6}, "summary": "fixture", "tags": []string{"method"}, "complexity": "simple"},
		map[string]any{"id": "function:a/methods.go:Beta.Ping", "type": "function", "name": "Beta.Ping", "filePath": "a/methods.go", "lineRange": []int{7, 7}, "summary": "fixture", "tags": []string{"method"}, "complexity": "simple"},
		map[string]any{"id": "function:a/methods.go:Solo", "type": "function", "name": "Solo", "filePath": "a/methods.go", "lineRange": []int{8, 8}, "summary": "fixture", "tags": []string{"method"}, "complexity": "simple"},
		map[string]any{"id": "function:a/methods.go:Free", "type": "function", "name": "Free", "filePath": "a/methods.go", "lineRange": []int{9, 9}, "summary": "fixture", "tags": []string{"function"}, "complexity": "simple"},
	)
	graph["edges"] = append(graph["edges"].([]any),
		map[string]any{"source": "file:a/methods.go", "target": "function:a/methods.go:Solo", "type": "contains", "direction": "forward", "weight": 1},
	)
	writeJSONFile(t, graphPath, graph)

	fingerprintPath := filepath.Join(root, ".understand-anything", "fingerprints.json")
	var fingerprints map[string]any
	readJSONFile(t, fingerprintPath, &fingerprints)
	files := fingerprints["files"].(map[string]any)
	methods := files["a/methods.go"].(map[string]any)
	methodSource := `package a

type Alpha struct{}
type Beta struct{}

func (Alpha) Ping() {}
func (*Beta) Ping() {}
func (Alpha) Solo() {}
func Free() {}
`
	methods["contentHash"] = fixtureContentHash(methodSource)
	methods["totalLines"] = float64(strings.Count(methodSource, "\n") + 1)
	methods["functions"] = []any{
		map[string]any{"name": "Ping", "params": []any{}, "exported": true, "lineCount": 1},
		map[string]any{"name": "Ping", "params": []any{}, "exported": true, "lineCount": 1},
		map[string]any{"name": "Solo", "params": []any{}, "exported": true, "lineCount": 1},
		map[string]any{"name": "Free", "params": []any{}, "exported": true, "lineCount": 1},
	}
	writeJSONFile(t, fingerprintPath, fingerprints)

	require.NoError(t, Normalize(root))

	var normalizedGraph map[string]any
	readJSONFile(t, graphPath, &normalizedGraph)
	nodes := normalizedGraph["nodes"].([]any)
	require.Equal(t, []string{"Alpha.Ping", "Alpha.Solo", "Beta.Ping", "Free"}, functionNodeNames(t, nodes, "a/methods.go"))
	require.Contains(t, graphEdgeKeys(t, normalizedGraph["edges"].([]any)), "file:a/methods.go\x00function:a/methods.go:Alpha.Solo\x00contains")

	var normalizedFingerprints map[string]any
	readJSONFile(t, fingerprintPath, &normalizedFingerprints)
	normalizedMethods := normalizedFingerprints["files"].(map[string]any)["a/methods.go"].(map[string]any)
	require.Equal(t, fixtureContentHash(methodSource), normalizedMethods["contentHash"])
	require.Equal(t, float64(strings.Count(methodSource, "\n")+1), normalizedMethods["totalLines"])
	functions := normalizedMethods["functions"].([]any)
	require.Equal(t, "Alpha", functions[0].(map[string]any)["receiver"])
	require.Equal(t, "Alpha.Ping", functions[0].(map[string]any)["qualifiedName"])
	require.Equal(t, "*Beta", functions[1].(map[string]any)["receiver"])
	require.Equal(t, "Beta.Ping", functions[1].(map[string]any)["qualifiedName"])
	require.Equal(t, "Alpha.Solo", functions[2].(map[string]any)["qualifiedName"])
	require.Equal(t, "Free", functions[3].(map[string]any)["qualifiedName"])
	require.NotContains(t, functions[3].(map[string]any), "receiver")
}

func TestNormalizeRejectsIncompleteFunctionLineRange(t *testing.T) {
	root := writeSingleFunctionGraphFixture(t, []int{2}, "Ping")
	require.ErrorContains(t, Normalize(root), "lineRange must contain exactly start and end")
}

func TestNormalizeRejectsFunctionEndLineMismatch(t *testing.T) {
	root := writeSingleFunctionGraphFixture(t, []int{2, 99}, "Ping")
	require.ErrorContains(t, Normalize(root), "lineRange [2,99] does not match AST [2,2]")
}

func TestNormalizeRejectsFunctionStartLineMismatch(t *testing.T) {
	root := writeSingleFunctionGraphFixture(t, []int{99, 99}, "Ping")
	require.ErrorContains(t, Normalize(root), "line 99 matches 0 declarations")
}

func TestNormalizeRejectsExtraFunctionLineRangeValues(t *testing.T) {
	root := writeSingleFunctionGraphFixture(t, []int{2, 2, 2}, "Ping")
	require.ErrorContains(t, Normalize(root), "lineRange must contain exactly start and end, got 3 values")
}

func TestNormalizeRejectsFingerprintFunctionCountMismatch(t *testing.T) {
	root := writeSingleFunctionGraphFixture(t, []int{2, 2}, "Ping")
	fingerprintPath := filepath.Join(root, ".understand-anything", "fingerprints.json")
	var fingerprints map[string]any
	readJSONFile(t, fingerprintPath, &fingerprints)
	fingerprints["files"].(map[string]any)["a/a.go"].(map[string]any)["functions"] = []any{}
	writeJSONFile(t, fingerprintPath, fingerprints)

	require.ErrorContains(t, Normalize(root), "fingerprint function count mismatch for a/a.go: graph=0 source=1")
}

func TestNormalizeRejectsFingerprintFunctionOrderMismatch(t *testing.T) {
	root := writeSingleFunctionGraphFixture(t, []int{2, 2}, "Wrong")
	require.ErrorContains(t, Normalize(root), `fingerprint function order mismatch for a/a.go[0]: got "Wrong", source has "Ping"`)
}

func TestNormalizeRejectsFingerprintContentHashMismatch(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	mutateGoFingerprint(t, root, "a/a.go", func(fingerprint map[string]any) {
		fingerprint["contentHash"] = strings.Repeat("0", sha256.Size*2)
	})
	require.ErrorContains(t, Normalize(root), "fingerprint contentHash mismatch for a/a.go")
}

func TestNormalizeRejectsFingerprintTotalLinesMismatch(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	mutateGoFingerprint(t, root, "a/a.go", func(fingerprint map[string]any) {
		fingerprint["totalLines"] = float64(99)
	})
	require.ErrorContains(t, Normalize(root), "fingerprint totalLines mismatch for a/a.go: got 99, source has 2")
}

func TestNormalizeRejectsFingerprintFunctionLineCountMismatch(t *testing.T) {
	root := writeSingleFunctionGraphFixture(t, []int{2, 2}, "Ping")
	mutateGoFingerprint(t, root, "a/a.go", func(fingerprint map[string]any) {
		fingerprint["functions"].([]any)[0].(map[string]any)["lineCount"] = float64(99)
	})
	require.ErrorContains(t, Normalize(root), "fingerprint function lineCount mismatch for a/a.go[0] Ping: got 99, source has 1")
}

func TestNormalizeRejectsScanSizeLinesMismatch(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	scanPath := filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json")
	var scan map[string]any
	readJSONFile(t, scanPath, &scan)
	files := scan["files"].([]any)
	files[0].(map[string]any)["sizeLines"] = float64(99)
	writeJSONFile(t, scanPath, scan)

	require.ErrorContains(t, Normalize(root), "scan sizeLines mismatch for a/a.go: got 99, source has 1")
}

func TestNormalizeRejectsAmbiguousBareGoCallsTarget(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/methods.go": `package a
type Alpha struct{}
type Beta struct{}
func (Alpha) Ping() {}
func (Beta) Ping() {}
`,
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["nodes"] = append(graph["nodes"].([]any),
		map[string]any{"id": "function:a/methods.go:Alpha.Ping", "type": "function", "name": "Alpha.Ping", "filePath": "a/methods.go", "lineRange": []int{4, 4}, "summary": "fixture", "tags": []string{"method"}, "complexity": "simple"},
		map[string]any{"id": "function:a/methods.go:Beta.Ping", "type": "function", "name": "Beta.Ping", "filePath": "a/methods.go", "lineRange": []int{5, 5}, "summary": "fixture", "tags": []string{"method"}, "complexity": "simple"},
	)
	graph["edges"] = append(graph["edges"].([]any),
		map[string]any{"source": "function:a/methods.go:Alpha.Ping", "target": "function:a/methods.go:Ping", "type": "calls", "direction": "forward", "weight": 0.8},
	)
	writeJSONFile(t, graphPath, graph)

	fingerprintPath := filepath.Join(root, ".understand-anything", "fingerprints.json")
	var fingerprints map[string]any
	readJSONFile(t, fingerprintPath, &fingerprints)
	fingerprints["files"].(map[string]any)["a/methods.go"].(map[string]any)["functions"] = []any{
		map[string]any{"name": "Ping", "params": []any{}, "exported": true, "lineCount": 1},
		map[string]any{"name": "Ping", "params": []any{}, "exported": true, "lineCount": 1},
	}
	writeJSONFile(t, fingerprintPath, fingerprints)

	err := Normalize(root)
	require.ErrorContains(t, err, "ambiguous Go calls edge target function:a/methods.go:Ping")
}

func TestNormalizeResolvesBareCallToCoexistingFreeFunction(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/functions.go": `package a
type T struct{}
func Ping() {}
func (T) Ping() {}
func Caller() {}
`,
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["nodes"] = append(graph["nodes"].([]any),
		map[string]any{"id": "function:a/functions.go:Ping", "type": "function", "name": "Ping", "filePath": "a/functions.go", "lineRange": []int{3, 3}, "summary": "fixture", "tags": []string{"function"}, "complexity": "simple"},
		map[string]any{"id": "function:a/functions.go:T.Ping", "type": "function", "name": "T.Ping", "filePath": "a/functions.go", "lineRange": []int{4, 4}, "summary": "fixture", "tags": []string{"method"}, "complexity": "simple"},
		map[string]any{"id": "function:a/functions.go:Caller", "type": "function", "name": "Caller", "filePath": "a/functions.go", "lineRange": []int{5, 5}, "summary": "fixture", "tags": []string{"function"}, "complexity": "simple"},
	)
	graph["edges"] = append(graph["edges"].([]any),
		map[string]any{"source": "function:a/functions.go:Caller", "target": "function:a/functions.go:Ping", "type": "calls", "direction": "forward", "weight": 0.8},
	)
	writeJSONFile(t, graphPath, graph)

	fingerprintPath := filepath.Join(root, ".understand-anything", "fingerprints.json")
	var fingerprints map[string]any
	readJSONFile(t, fingerprintPath, &fingerprints)
	fingerprints["files"].(map[string]any)["a/functions.go"].(map[string]any)["functions"] = []any{
		map[string]any{"name": "Ping", "params": []any{}, "exported": true, "lineCount": 1},
		map[string]any{"name": "Ping", "params": []any{}, "exported": true, "lineCount": 1},
		map[string]any{"name": "Caller", "params": []any{}, "exported": true, "lineCount": 1},
	}
	writeJSONFile(t, fingerprintPath, fingerprints)

	require.NoError(t, Normalize(root))
	var normalized graphFixture
	readJSONFile(t, graphPath, &normalized)
	require.Contains(t, normalized.EdgeKeys(), "function:a/functions.go:Caller\x00function:a/functions.go:Ping\x00calls")
}

func TestNormalizeRejectsInvalidGoStructure(t *testing.T) {
	tests := []struct {
		name    string
		sources map[string]string
		want    string
	}{
		{
			name: "syntax error",
			sources: map[string]string{
				"a/a.go": "package a\nfunc (\n",
			},
			want: "parse Go source a/a.go",
		},
		{
			name: "multiple production packages",
			sources: map[string]string{
				"a/a.go": "package a\n",
				"a/b.go": "package b\n",
			},
			want: "contains multiple production packages",
		},
		{
			name: "unsupported receiver",
			sources: map[string]string{
				"a/a.go": "package a\nfunc (interface{}) Ping() {}\n",
			},
			want: "unsupported receiver type",
		},
		{
			name: "missing internal package",
			sources: map[string]string{
				"a/a.go": "package a\nimport _ \"example.com/pixiv/missing\"\n",
			},
			want: "imports missing internal package example.com/pixiv/missing",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.sources["go.mod"] = "module example.com/pixiv\n\ngo 1.26.3\n"
			root := writeGraphFixture(t, test.sources)
			require.ErrorContains(t, Normalize(root), test.want)
		})
	}
}

func TestNormalizeIsByteIdenticalAndPreservesNonGoAndDocsData(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod":               "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go":               "package a\n",
		"consumer/consumer.go": "package consumer\n\nimport _ \"example.com/pixiv/a\"\n",
	})
	scanPath := filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json")
	var scan map[string]any
	readJSONFile(t, scanPath, &scan)
	scan["files"] = append(scan["files"].([]any), map[string]any{"path": "web/app.ts", "language": "typescript", "sizeLines": 1, "fileCategory": "code"})
	scan["importMap"].(map[string]any)["web/app.ts"] = []any{"web/dependency.ts"}
	scan["customField"] = map[string]any{"preserve": true}
	writeJSONFile(t, scanPath, scan)

	docsPath := filepath.Join(root, "docs", ".understand-anything", "knowledge-graph.json")
	docsBefore := []byte("{\n  \"mustRemain\": \"byte-identical\"\n}\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(docsPath), 0o755))
	require.NoError(t, os.WriteFile(docsPath, docsBefore, 0o600))

	require.NoError(t, Normalize(root))
	paths := []string{
		scanPath,
		filepath.Join(root, ".understand-anything", "knowledge-graph.json"),
		filepath.Join(root, ".understand-anything", "fingerprints.json"),
	}
	first := make(map[string][]byte, len(paths))
	for _, path := range paths {
		first[path] = readFile(t, path)
	}
	require.NoError(t, Normalize(root))
	for _, path := range paths {
		require.Equal(t, first[path], readFile(t, path), "second normalization changed %s", path)
	}
	require.Equal(t, docsBefore, readFile(t, docsPath))

	var normalizedScan map[string]any
	readJSONFile(t, scanPath, &normalizedScan)
	require.Equal(t, []string{"web/dependency.ts"}, stringSliceMap(t, normalizedScan["importMap"])["web/app.ts"])
	require.Equal(t, map[string]any{"preserve": true}, normalizedScan["customField"])
}

func TestNormalizeRemovesOwnedModulesForDeletedPackages(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["nodes"] = append(graph["nodes"].([]any),
		map[string]any{"id": "module:example.com/pixiv/removed", "type": "module", "name": "removed", "summary": "stale", "tags": []string{"go-package", "module"}, "complexity": "simple"},
		map[string]any{"id": "module:external-system", "type": "module", "name": "external", "summary": "not owned", "tags": []string{"module"}, "complexity": "simple"},
	)
	graph["edges"] = append(graph["edges"].([]any),
		map[string]any{"source": "module:example.com/pixiv/removed", "target": "file:a/a.go", "type": "contains", "direction": "forward", "weight": 1},
		map[string]any{"source": "module:external-system", "target": "file:a/a.go", "type": "contains", "direction": "forward", "weight": 1},
	)
	writeJSONFile(t, graphPath, graph)

	require.NoError(t, Normalize(root))
	var normalized graphFixture
	readJSONFile(t, graphPath, &normalized)
	require.NotContains(t, normalized.NodeIDs(), "module:example.com/pixiv/removed")
	require.NotContains(t, normalized.EdgeKeys(), "module:example.com/pixiv/removed\x00file:a/a.go\x00contains")
	require.Contains(t, normalized.NodeIDs(), "module:external-system")
	require.Contains(t, normalized.EdgeKeys(), "module:external-system\x00file:a/a.go\x00contains")
}

func TestNormalizeRejectsPackageIDOwnedByUnrelatedModule(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["nodes"] = append(graph["nodes"].([]any),
		map[string]any{"id": "module:example.com/pixiv/a", "type": "module", "name": "unrelated", "summary": "not owned", "tags": []string{"module"}, "complexity": "simple"},
	)
	writeJSONFile(t, graphPath, graph)

	err := Normalize(root)
	require.ErrorContains(t, err, "Go package module ID module:example.com/pixiv/a conflicts with non-owned graph node")
}

func TestNormalizeRejectsMixedOwnershipDuplicateInputNodeID(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["nodes"] = append(graph["nodes"].([]any),
		map[string]any{"id": "module:example.com/pixiv/stale", "type": "module", "name": "owned", "summary": "owned", "tags": []string{"go-package", "module"}, "complexity": "simple"},
		map[string]any{"id": "module:example.com/pixiv/stale", "type": "module", "name": "foreign", "summary": "not owned", "tags": []string{"module"}, "complexity": "simple"},
	)
	writeJSONFile(t, graphPath, graph)

	require.ErrorContains(t, Normalize(root), "input graph contains duplicate node ID module:example.com/pixiv/stale")
}

func TestNormalizeRejectsDanglingGraphEdge(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["edges"] = append(graph["edges"].([]any),
		map[string]any{"source": "file:a/a.go", "target": "function:a/a.go:Missing", "type": "calls", "direction": "forward", "weight": 0.8},
	)
	writeJSONFile(t, graphPath, graph)

	require.ErrorContains(t, Normalize(root), "graph edge file:a/a.go -> function:a/a.go:Missing (calls) has missing target node")
}

func TestNormalizePreservesUnknownGraphNodeAndEdgeFields(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["nodes"] = append(graph["nodes"].([]any), map[string]any{
		"id": "file:web/app.ts", "type": "file", "name": "app.ts", "filePath": "web/app.ts",
		"summary": "non-Go fixture", "tags": []any{"typescript"}, "complexity": "simple",
		"domainMeta":    map[string]any{"boundedContext": "web"},
		"knowledgeMeta": map[string]any{"confidence": 0.75},
		"unknownScalar": "keep-me", "unknownNested": map[string]any{"items": []any{1.0, "two"}},
	})
	graph["edges"] = append(graph["edges"].([]any), map[string]any{
		"source": "file:web/app.ts", "target": "file:web/app.ts", "type": "references",
		"direction": "bidirectional", "description": "known metadata", "weight": 0.25,
		"unknownScalar": true, "unknownNested": map[string]any{"source": "plugin"},
	})
	writeJSONFile(t, graphPath, graph)

	require.NoError(t, Normalize(root))
	var normalized map[string]any
	readJSONFile(t, graphPath, &normalized)
	node := graphObjectByID(t, normalized["nodes"].([]any), "file:web/app.ts")
	require.Equal(t, "keep-me", node["unknownScalar"])
	require.Equal(t, map[string]any{"items": []any{1.0, "two"}}, node["unknownNested"])
	require.Equal(t, map[string]any{"boundedContext": "web"}, node["domainMeta"])
	require.Equal(t, map[string]any{"confidence": 0.75}, node["knowledgeMeta"])
	edge := graphEdgeObject(t, normalized["edges"].([]any), "file:web/app.ts", "file:web/app.ts", "references")
	require.Equal(t, true, edge["unknownScalar"])
	require.Equal(t, map[string]any{"source": "plugin"}, edge["unknownNested"])
	require.Equal(t, "bidirectional", edge["direction"])
	require.Equal(t, "known metadata", edge["description"])
	require.Equal(t, 0.25, edge["weight"])
}

func TestNormalizeRejectsConflictingDuplicateGraphEdges(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["nodes"] = append(graph["nodes"].([]any),
		map[string]any{"id": "file:web/a.ts", "type": "file", "name": "a.ts", "filePath": "web/a.ts", "summary": "fixture", "tags": []any{"typescript"}, "complexity": "simple"},
		map[string]any{"id": "file:web/b.ts", "type": "file", "name": "b.ts", "filePath": "web/b.ts", "summary": "fixture", "tags": []any{"typescript"}, "complexity": "simple"},
	)
	graph["edges"] = append(graph["edges"].([]any),
		map[string]any{"source": "file:web/a.ts", "target": "file:web/b.ts", "type": "references", "direction": "forward", "weight": 0.5, "pluginMeta": map[string]any{"rank": 1.0}},
		map[string]any{"source": "file:web/a.ts", "target": "file:web/b.ts", "type": "references", "direction": "bidirectional", "weight": 0.5, "pluginMeta": map[string]any{"rank": 2.0}},
	)
	writeJSONFile(t, graphPath, graph)

	require.ErrorContains(t, Normalize(root), "conflicting duplicate graph edge file:web/a.ts -> file:web/b.ts (references)")
}

func TestNormalizeFoldsOnlyIdenticalDuplicateGraphEdges(t *testing.T) {
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\n",
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	edge := map[string]any{
		"source": "file:a/a.go", "target": "file:a/a.go", "type": "references",
		"direction": "forward", "description": "same", "weight": 0.5,
		"pluginMeta": map[string]any{"rank": 1.0},
	}
	graph["edges"] = append(graph["edges"].([]any), edge, edge)
	writeJSONFile(t, graphPath, graph)

	require.NoError(t, Normalize(root))
	var normalized graphFixture
	readJSONFile(t, graphPath, &normalized)
	count := 0
	for _, key := range normalized.EdgeKeys() {
		if key == "file:a/a.go\x00file:a/a.go\x00references" {
			count++
		}
	}
	require.Equal(t, 1, count)
}

type graphFixture struct {
	Nodes []struct {
		ID string `json:"id"`
	} `json:"nodes"`
	Edges []struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Type   string `json:"type"`
	} `json:"edges"`
}

func (g graphFixture) NodeIDs() []string {
	ids := make([]string, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func (g graphFixture) EdgeKeys() []string {
	keys := make([]string, 0, len(g.Edges))
	for _, edge := range g.Edges {
		keys = append(keys, edge.Source+"\x00"+edge.Target+"\x00"+edge.Type)
	}
	return keys
}

func writeGraphFixture(t *testing.T, sources map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range sources {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	files := []map[string]any{}
	fileNodes := []map[string]any{}
	importMap := map[string][]string{}
	fingerprints := map[string]any{}
	goFiles := make([]string, 0)
	for name := range sources {
		if filepath.Ext(name) == ".go" {
			goFiles = append(goFiles, name)
		}
	}
	sort.Strings(goFiles)
	for _, name := range goFiles {
		content := sources[name]
		files = append(files, map[string]any{"path": name, "language": "go", "sizeLines": strings.Count(content, "\n"), "fileCategory": "code"})
		fileNodes = append(fileNodes, map[string]any{
			"id": "file:" + name, "type": "file", "name": filepath.Base(name), "filePath": name,
			"summary": "fixture", "tags": []string{"test"}, "complexity": "simple",
		})
		importMap[name] = []string{}
		fingerprints[name] = map[string]any{
			"filePath": name, "contentHash": fixtureContentHash(content), "functions": []any{}, "classes": []any{},
			"imports": []any{}, "exports": []any{}, "totalLines": strings.Count(content, "\n") + 1, "hasStructuralAnalysis": true,
		}
	}
	expanded := []string{"a/a.go", "a/a_more.go", "a/a_test.go", "a/external_test.go"}
	for _, source := range []string{"consumer/consumer.go", "a/external_test.go"} {
		if _, ok := importMap[source]; !ok {
			continue
		}
		importMap[source] = expanded
		fingerprints[source].(map[string]any)["imports"] = []map[string]any{{"source": "example.com/pixiv/a", "specifiers": []string{"_"}}}
	}

	edges := []map[string]any{}
	for _, source := range []string{"consumer/consumer.go", "a/external_test.go"} {
		if _, ok := importMap[source]; !ok {
			continue
		}
		for _, target := range expanded {
			edges = append(edges, map[string]any{"source": "file:" + source, "target": "file:" + target, "type": "imports", "direction": "forward", "weight": 0.7})
		}
	}
	writeJSONFile(t, filepath.Join(root, ".understand-anything", "intermediate", "scan-result.json"), map[string]any{"files": files, "importMap": importMap})
	writeJSONFile(t, filepath.Join(root, ".understand-anything", "knowledge-graph.json"), map[string]any{
		"version": "1.0.0", "project": map[string]any{"name": "fixture"}, "nodes": fileNodes, "edges": edges,
		"layers": []any{}, "tour": []any{},
	})
	writeJSONFile(t, filepath.Join(root, ".understand-anything", "fingerprints.json"), map[string]any{
		"version": "1.0.0", "gitCommitHash": "fixture", "generatedAt": "fixed", "files": fingerprints,
	})
	return root
}

func functionNodeNames(t *testing.T, nodes []any, filePath string) []string {
	t.Helper()
	var names []string
	for _, value := range nodes {
		node := value.(map[string]any)
		if node["type"] == "function" && node["filePath"] == filePath {
			names = append(names, node["name"].(string))
		}
	}
	sort.Strings(names)
	return names
}

func graphEdgeKeys(t *testing.T, edges []any) []string {
	t.Helper()
	keys := make([]string, 0, len(edges))
	for _, value := range edges {
		edge := value.(map[string]any)
		keys = append(keys, edge["source"].(string)+"\x00"+edge["target"].(string)+"\x00"+edge["type"].(string))
	}
	return keys
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func fixtureContentHash(content string) string {
	digest := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", digest)
}

func stagedJSONArtifacts(t *testing.T, root string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, ".*.tmp-*"))
	require.NoError(t, err)
	return matches
}

func graphObjectByID(t *testing.T, objects []any, id string) map[string]any {
	t.Helper()
	for _, value := range objects {
		object := value.(map[string]any)
		if object["id"] == id {
			return object
		}
	}
	t.Fatalf("graph node %s is missing", id)
	return nil
}

func graphEdgeObject(t *testing.T, objects []any, source, target, edgeType string) map[string]any {
	t.Helper()
	for _, value := range objects {
		object := value.(map[string]any)
		if object["source"] == source && object["target"] == target && object["type"] == edgeType {
			return object
		}
	}
	t.Fatalf("graph edge %s -> %s (%s) is missing", source, target, edgeType)
	return nil
}

func writeSingleFunctionGraphFixture(t *testing.T, lineRange []int, fingerprintName string) string {
	t.Helper()
	root := writeGraphFixture(t, map[string]string{
		"go.mod": "module example.com/pixiv\n\ngo 1.26.3\n",
		"a/a.go": "package a\nfunc Ping() {}\n",
	})
	graphPath := filepath.Join(root, ".understand-anything", "knowledge-graph.json")
	var graph map[string]any
	readJSONFile(t, graphPath, &graph)
	graph["nodes"] = append(graph["nodes"].([]any),
		map[string]any{"id": "function:a/a.go:Ping", "type": "function", "name": "Ping", "filePath": "a/a.go", "lineRange": lineRange, "summary": "fixture", "tags": []string{"function"}, "complexity": "simple"},
	)
	writeJSONFile(t, graphPath, graph)

	fingerprintPath := filepath.Join(root, ".understand-anything", "fingerprints.json")
	var fingerprints map[string]any
	readJSONFile(t, fingerprintPath, &fingerprints)
	fingerprints["files"].(map[string]any)["a/a.go"].(map[string]any)["functions"] = []any{
		map[string]any{"name": fingerprintName, "params": []any{}, "exported": true, "lineCount": 1},
	}
	writeJSONFile(t, fingerprintPath, fingerprints)
	return root
}

func mutateGoFingerprint(t *testing.T, root, filePath string, mutate func(map[string]any)) {
	t.Helper()
	path := filepath.Join(root, ".understand-anything", "fingerprints.json")
	var fingerprints map[string]any
	readJSONFile(t, path, &fingerprints)
	fingerprint := fingerprints["files"].(map[string]any)[filePath].(map[string]any)
	mutate(fingerprint)
	writeJSONFile(t, path, fingerprints)
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	data, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600))
}

func readJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, value))
}

func stringSliceMap(t *testing.T, value any) map[string][]string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	var result map[string][]string
	require.NoError(t, json.Unmarshal(encoded, &result))
	return result
}
