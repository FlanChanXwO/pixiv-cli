package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
