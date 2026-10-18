package nativeevidence

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsolidateEvidencePublishesOnlyCompleteSameSourceGoArtifacts(t *testing.T) {
	workDir := newRepositoryWorkDir(t)
	inputDir := filepath.Join(workDir, "evidence")
	if err := os.Mkdir(inputDir, 0o755); err != nil {
		t.Fatalf("create evidence directory: %v", err)
	}
	for platform, target := range nativeTargets {
		writeConsolidationFixture(t, inputDir, platform, target)
	}
	outputDir := filepath.Join(workDir, "verified")
	if err := consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: inputDir, outputDir: outputDir}); err != nil {
		t.Fatalf("consolidate six native records: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(outputDir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var summary evidenceSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Schema != 3 || summary.SourceCommit != testEvidenceCommit || summary.Version != "v0.1.0-native-evidence.test" || len(summary.Records) != 6 {
		t.Fatalf("incomplete consolidated Go evidence: %+v", summary)
	}
	for _, record := range summary.Records {
		platform := record.Target.GOOS + "-" + record.Target.GOARCH
		for _, name := range []string{record.Binary.Name, record.Archive.Name, "native-evidence.json"} {
			if err := requireRegularFile(filepath.Join(outputDir, platform, name), "copied evidence"); err != nil {
				t.Fatal(err)
			}
		}
		source, err := os.Stat(filepath.Join(inputDir, platform, record.Binary.Name))
		if err != nil {
			t.Fatal(err)
		}
		copied, err := os.Stat(filepath.Join(outputDir, platform, record.Binary.Name))
		if err != nil {
			t.Fatal(err)
		}
		if copied.Mode().Perm() != source.Mode().Perm() {
			t.Fatalf("copied binary permissions = %v, want %v", copied.Mode().Perm(), source.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, "manifest.json")); !os.IsNotExist(err) {
		t.Fatal("consolidation retained the Rust staticlib manifest")
	}
}

func TestConsolidateEvidenceRejectsMissingTargetWithoutPublishingOutput(t *testing.T) {
	workDir := newRepositoryWorkDir(t)
	inputDir := filepath.Join(workDir, "evidence")
	if err := os.Mkdir(inputDir, 0o755); err != nil {
		t.Fatalf("create evidence directory: %v", err)
	}
	for platform, target := range nativeTargets {
		if platform == "windows/arm64" {
			continue
		}
		writeConsolidationFixture(t, inputDir, platform, target)
	}
	outputDir := filepath.Join(workDir, "verified")
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
	for platform, target := range nativeTargets {
		writeConsolidationFixture(t, inputDir, platform, target)
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
	outputDir := filepath.Join(workDir, "verified")
	err = consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: inputDir, outputDir: outputDir})
	if err == nil || !strings.Contains(err.Error(), "members") {
		t.Fatalf("consolidate error = %v, want archive-member mismatch rejection", err)
	}
	if _, statErr := os.Lstat(outputDir); !os.IsNotExist(statErr) {
		t.Fatalf("failed consolidation published output: %v", statErr)
	}
}

func TestConsolidateEvidenceRejectsMixedSourceOrBinaryVersionWithoutPublishingOutput(t *testing.T) {
	for _, mutation := range []struct {
		name      string
		wantError string
		apply     func(*evidenceRecord)
	}{
		{name: "schema", wantError: "unsupported schema", apply: func(record *evidenceRecord) { record.Schema = 2 }},
		{name: "valid but wrong commit", wantError: "source metadata", apply: func(record *evidenceRecord) { record.SourceCommit = strings.Repeat("a", 40) }},
		{name: "duplicate target", wantError: "duplicate target", apply: func(record *evidenceRecord) { record.Target = evidenceTarget{GOOS: "darwin", GOARCH: "amd64"} }},
		{name: "unsupported target", wantError: "unsupported target", apply: func(record *evidenceRecord) { record.Target.GOARCH = "mips" }},
		{name: "binary hash", wantError: "binary", apply: func(record *evidenceRecord) { record.Binary.SHA256 = strings.Repeat("e", 64) }},
		{name: "archive hash", wantError: "archive", apply: func(record *evidenceRecord) { record.Archive.SHA256 = strings.Repeat("e", 64) }},
		{name: "binary traversal", wantError: "binary", apply: func(record *evidenceRecord) { record.Binary.Name = "../pixiv" }},
		{name: "archive member traversal", wantError: "members", apply: func(record *evidenceRecord) { record.Archive.Members[0].Name = "../LICENSE" }},
		{name: "archive member duplicate", wantError: "members", apply: func(record *evidenceRecord) {
			record.Archive.Members = append(record.Archive.Members, record.Archive.Members[0])
		}},
		{name: "commit", wantError: "source metadata", apply: func(record *evidenceRecord) { record.SourceCommit = "other-main-sha" }},
		{name: "version", wantError: "binary version", apply: func(record *evidenceRecord) { record.Binary.Version = "v0.1.0-native-evidence.other" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			workDir := newRepositoryWorkDir(t)
			inputDir := filepath.Join(workDir, "evidence")
			if err := os.Mkdir(inputDir, 0o755); err != nil {
				t.Fatalf("create evidence directory: %v", err)
			}
			for platform, target := range nativeTargets {
				writeConsolidationFixture(t, inputDir, platform, target)
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
			outputDir := filepath.Join(workDir, "verified")
			err = consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: inputDir, outputDir: outputDir})
			if err == nil || !strings.Contains(err.Error(), mutation.wantError) {
				t.Fatalf("consolidate error = %v, want %q rejection", err, mutation.wantError)
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
			outputDir := filepath.Join(workDir, "verified")
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
	for _, missing := range []string{"pixiv"} {
		t.Run(missing, func(t *testing.T) {
			workDir := newRepositoryWorkDir(t)
			inputDir := filepath.Join(workDir, "evidence")
			if err := os.Mkdir(inputDir, 0o755); err != nil {
				t.Fatalf("create evidence directory: %v", err)
			}
			for platform, target := range nativeTargets {
				writeConsolidationFixture(t, inputDir, platform, target)
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
			outputDir := filepath.Join(workDir, "verified")
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

func TestConsolidateRejectsLegacyFieldsAndSymlinkArtifacts(t *testing.T) {
	for _, mutation := range []string{"source_digest", "staticlib", "artifact symlink"} {
		t.Run(mutation, func(t *testing.T) {
			work := newRepositoryWorkDir(t)
			input := filepath.Join(work, "input")
			if err := os.Mkdir(input, 0o755); err != nil {
				t.Fatal(err)
			}
			for platform, target := range nativeTargets {
				writeConsolidationFixture(t, input, platform, target)
			}
			path := filepath.Join(input, "linux-amd64", "native-evidence.json")
			if mutation == "artifact symlink" {
				path = filepath.Join(input, "linux-amd64", "pixiv")
				if err := os.Rename(path, filepath.Join(work, "original-binary")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(work, "original-binary"), path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			} else {
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var record map[string]any
				if err := json.Unmarshal(body, &record); err != nil {
					t.Fatal(err)
				}
				record[mutation] = "legacy"
				body, err = json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, body, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			output := filepath.Join(work, "output")
			if err := consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: input, outputDir: output}); err == nil {
				t.Fatal("legacy metadata or symlink was accepted")
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("rejected evidence published output: %v", err)
			}
		})
	}
}

func TestConsolidateRejectsRehashedArchiveWithTamperedBinary(t *testing.T) {
	work := newRepositoryWorkDir(t)
	input := filepath.Join(work, "input")
	if err := os.Mkdir(input, 0o755); err != nil {
		t.Fatal(err)
	}
	for platform, target := range nativeTargets {
		writeConsolidationFixture(t, input, platform, target)
	}
	path := filepath.Join(input, "windows-amd64", "native-evidence.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record evidenceRecord
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(filepath.Dir(path), record.Archive.Name)
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	var modified bytes.Buffer
	writer := zip.NewWriter(&modified)
	tampered := []byte("changed binary, with self-consistent archive hashes")
	for _, entry := range reader.File {
		destination, err := writer.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		source, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name == "pixiv.exe" {
			_, err = destination.Write(tampered)
		} else {
			_, err = io.Copy(destination, source)
		}
		closeErr := source.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("rewrite archive: %v %v", err, closeErr)
		}
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, modified.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	record.Archive.SHA256, err = fileSHA256(archive)
	if err != nil {
		t.Fatal(err)
	}
	for i := range record.Archive.Members {
		if record.Archive.Members[i].Name == "pixiv.exe" {
			record.Archive.Members[i].SHA256 = fmt.Sprintf("%x", sha256.Sum256(tampered))
		}
	}
	body, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(work, "output")
	err = consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: input, outputDir: output})
	if err == nil || !strings.Contains(err.Error(), "audited tree") {
		t.Fatalf("self-consistent tampered evidence must fail the source-tree comparison: %v", err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("tampered evidence published output: %v", err)
	}
}

func TestConsolidateRejectsArchiveWithUnsafeDirectory(t *testing.T) {
	work := newRepositoryWorkDir(t)
	input := filepath.Join(work, "input")
	if err := os.Mkdir(input, 0o755); err != nil {
		t.Fatal(err)
	}
	for platform, target := range nativeTargets {
		writeConsolidationFixture(t, input, platform, target)
	}
	path := filepath.Join(input, "windows-amd64", "native-evidence.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record evidenceRecord
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(filepath.Dir(path), record.Archive.Name)
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range reader.File {
		if err := writer.Copy(entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	header := &zip.FileHeader{Name: "../outside/", Method: zip.Store}
	header.SetMode(os.ModeDir | 0o755)
	if _, err := writer.CreateHeader(header); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	record.Archive.SHA256, err = fileSHA256(archive)
	if err != nil {
		t.Fatal(err)
	}
	body, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(work, "output")
	err = consolidateEvidence(consolidateOptions{repoRoot: findRepositoryRoot(t), expectedVersion: "v0.1.0-native-evidence.test", expectedCommit: testEvidenceCommit, inputDir: input, outputDir: output})
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("consolidate error = %v, want unsafe directory rejection", err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("unsafe archive published output: %v", err)
	}
}
