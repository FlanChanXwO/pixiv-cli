package nativeevidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		repoRoot:     repoRoot,
		version:      "0.1.0-native-evidence.test",
		sourceCommit: testEvidenceCommit,
		target:       "linux/amd64",
		rustTarget:   "x86_64-unknown-linux-gnu",
		staticlib:    staticlib,
		binary:       binary,
		archive:      archive,
		output:       output,
	})
	if err != nil {
		t.Fatalf("record native evidence: %v", err)
	}
	if record.Binary.Version != "v0.1.0-native-evidence.test" {
		t.Fatalf("binary version = %q", record.Binary.Version)
	}
	if record.SourceCommit != testEvidenceCommit {
		t.Fatalf("source commit = %q", record.SourceCommit)
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
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &encoded); err != nil {
		t.Fatalf("decode evidence fields: %v", err)
	}
	var binaryFields map[string]json.RawMessage
	if err := json.Unmarshal(encoded["binary"], &binaryFields); err != nil {
		t.Fatalf("decode binary evidence fields: %v", err)
	}
	if len(binaryFields) != 3 || binaryFields["name"] == nil || binaryFields["sha256"] == nil || binaryFields["version"] == nil {
		t.Fatalf("binary evidence fields = %v, want name, sha256 and version only", binaryFields)
	}
	if encoded["source_commit"] == nil {
		t.Fatalf("evidence fields = %v, want independent source_commit", encoded)
	}
}

func TestReadBinaryVersionRequiresExactRootOutput(t *testing.T) {
	binaryName := "pixiv"
	if runtime.GOOS == "windows" {
		binaryName = "pixiv.exe"
	}
	binary := filepath.Join(t.TempDir(), binaryName)
	copyTestExecutable(t, binary)

	tests := []struct {
		name   string
		output string
		fail   bool
		want   string
	}{
		{name: "exact", output: "pixiv v0.1.0-native-evidence.test\n", want: "v0.1.0-native-evidence.test"},
		{name: "wrong version", output: "pixiv v0.1.0-native-evidence.other\n"},
		{name: "missing newline", output: "pixiv v0.1.0-native-evidence.test"},
		{name: "trailing data", output: "pixiv v0.1.0-native-evidence.test\nextra\n"},
		{name: "legacy json", output: `{"version":"v0.1.0-native-evidence.test"}` + "\n"},
		{name: "process failure", fail: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(testBinaryVersionOutputEnv, test.output)
			if test.fail {
				t.Setenv(testBinaryVersionFailEnv, "1")
			}
			got, err := readBinaryVersion(binary, "v0.1.0-native-evidence.test")
			if test.want == "" {
				if err == nil {
					t.Fatalf("readBinaryVersion() = %q, want error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("readBinaryVersion() = %q, %v, want %q", got, err, test.want)
			}
		})
	}
}

func TestRecordEvidenceRejectsMalformedSourceCommitBeforeFilesystem(t *testing.T) {
	for _, commit := range []string{"", "abc", strings.Repeat("a", 39), strings.Repeat("a", 41), strings.Repeat("A", 40)} {
		t.Run("commit="+commit, func(t *testing.T) {
			_, err := recordEvidence(recordOptions{version: "0.1.0-native-evidence.test", sourceCommit: commit})
			if err == nil || !strings.Contains(err.Error(), "source commit") {
				t.Fatalf("recordEvidence() error = %v, want source commit rejection", err)
			}
		})
	}
}
