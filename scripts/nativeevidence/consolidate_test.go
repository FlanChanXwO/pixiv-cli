package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/download/staticlib"
)

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
	if err := staticlib.ValidateManifestFiles(outputDir, manifest, sourceDigest); err != nil {
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
