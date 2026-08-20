package nativeevidence

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const testEvidenceCommit = "0123456789abcdef0123456789abcdef01234567"

const (
	testBinaryVersionOutputEnv = "PIXIV_NATIVE_EVIDENCE_TEST_VERSION_OUTPUT"
	testBinaryVersionFailEnv   = "PIXIV_NATIVE_EVIDENCE_TEST_VERSION_FAIL"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		if os.Getenv(testBinaryVersionFailEnv) != "" {
			os.Exit(17)
		}
		output, configured := os.LookupEnv(testBinaryVersionOutputEnv)
		if !configured {
			output = "pixiv v0.1.0-native-evidence.test\n"
		}
		_, _ = os.Stdout.WriteString(output)
		os.Exit(0)
	}
	os.Exit(m.Run())
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
		Schema:       evidenceSchemaVersion,
		Target:       evidenceTarget{GOOS: target.goos, GOARCH: target.goarch, RustTarget: target.rustTarget},
		SourceCommit: testEvidenceCommit,
		SourceDigest: sourceDigest,
		Staticlib:    evidenceFile{Name: target.staticlib, SHA256: staticlibDigest},
		Binary:       evidenceBinary{evidenceFile: evidenceFile{Name: binaryName, SHA256: binaryDigest}, Version: "v0.1.0-native-evidence.test"},
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
	command := exec.Command("go", "run", "./scripts/cmd/releaseassets", "package",
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
