package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestValidateRejectsNonSemanticVersion(t *testing.T) {
	t.Parallel()

	if err := run([]string{"validate", "--version", "0.1.0-beta.1"}); err != nil {
		t.Fatalf("validate accepted semantic version: %v", err)
	}
	err := run([]string{"validate", "--version", "1not-semver"})
	if err == nil || !strings.Contains(err.Error(), "semantic version") {
		t.Fatalf("validate error = %v, want semantic-version rejection", err)
	}
}

func TestValidateSourceRequiresMatchingProductSkillVersion(t *testing.T) {
	t.Parallel()

	err := run([]string{"validate-source", "--version", "0.1.0"})
	if err == nil || !strings.Contains(err.Error(), "--product-skill is required") {
		t.Fatalf("validate-source without product skill = %v, want required-flag error", err)
	}

	workDir := newTestWorkDir(t)
	skill := filepath.Join(workDir, "SKILL.md")
	if err := os.WriteFile(skill, []byte("---\n# Product metadata\nversion: \"0.1.0\"\nslug: pixiv-cli\n---\n"), 0o644); err != nil {
		t.Fatalf("write product skill fixture: %v", err)
	}
	if err := run([]string{"validate-source", "--version", "0.1.0", "--product-skill", skill}); err != nil {
		t.Fatalf("validate matching product skill source: %v", err)
	}

	err = run([]string{"validate-source", "--version", "0.1.1", "--product-skill", skill})
	if err == nil || !strings.Contains(err.Error(), "product skill version") {
		t.Fatalf("validate mismatched product skill = %v, want version rejection", err)
	}
}

func TestValidateSourceRejectsInvalidProductSkillMetadata(t *testing.T) {
	t.Parallel()

	for name, contents := range map[string]string{
		"missing version":   "---\nslug: pixiv-cli\n---\n",
		"duplicate version": "---\nversion: 0.1.0\nversion: 0.1.0\n---\n",
		"missing delimiter": "---\nversion: 0.1.0\n",
	} {
		t.Run(name, func(t *testing.T) {
			workDir := newTestWorkDir(t)
			skill := filepath.Join(workDir, "SKILL.md")
			if err := os.WriteFile(skill, []byte(contents), 0o644); err != nil {
				t.Fatalf("write product skill fixture: %v", err)
			}
			if err := run([]string{"validate-source", "--version", "0.1.0", "--product-skill", skill}); err == nil {
				t.Fatal("validate-source accepted invalid product skill metadata")
			}
		})
	}
}

func TestChannelClassifiesSemanticPrereleaseInsteadOfBuildMetadata(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		version string
		want    string
	}{
		{name: "stable build metadata", version: "0.1.0+build-1", want: "stable"},
		{name: "prerelease with build metadata", version: "0.1.0-rc.1+build-1", want: "prerelease"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := channelOutput([]string{"channel", "--version", test.version})
			if err != nil {
				t.Fatalf("classify %q: %v", test.version, err)
			}
			if output != test.want+"\n" {
				t.Fatalf("channel output = %q, want %q", output, test.want+"\n")
			}
		})
	}
}

func TestPackageCreatesFixedTargetArchive(t *testing.T) {
	t.Parallel()

	repoRoot := findTestRepositoryRoot(t)
	workDir, err := os.MkdirTemp(repoRoot, ".pixiv-releaseassets-test.")
	if err != nil {
		t.Fatalf("create repository-local temporary directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(workDir); err != nil {
			t.Errorf("remove temporary directory: %v", err)
		}
	})
	binary := filepath.Join(workDir, "pixiv")
	if err := os.WriteFile(binary, []byte("fixture executable"), 0o755); err != nil {
		t.Fatalf("write fixture binary: %v", err)
	}
	outputDir := filepath.Join(workDir, "assets")
	if err := os.Mkdir(outputDir, 0o755); err != nil {
		t.Fatalf("create output directory: %v", err)
	}

	if err := run([]string{
		"package",
		"--repo-root", repoRoot,
		"--version", "0.1.0",
		"--target", "linux/amd64",
		"--binary", binary,
		"--output-dir", outputDir,
	}); err != nil {
		t.Fatalf("package release asset: %v", err)
	}

	archive := filepath.Join(outputDir, "pixiv-cli_0.1.0_linux_amd64.tar.gz")
	info, err := os.Lstat(archive)
	if err != nil {
		t.Fatalf("stat fixed-name archive: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("archive must be a regular file, got %s", info.Mode())
	}
}

func TestFinalizeBuildsSignedReleaseDirectory(t *testing.T) {
	t.Parallel()

	workDir := newTestWorkDir(t)
	inputDir := filepath.Join(workDir, "input")
	if err := os.Mkdir(inputDir, 0o755); err != nil {
		t.Fatalf("create input directory: %v", err)
	}
	version := "0.1.0"
	archiveContents := writeArchiveFixtures(t, inputDir, version)
	installSh := filepath.Join(workDir, "install.sh")
	installCmd := filepath.Join(workDir, "install.cmd")
	installerContents := map[string]string{
		"install.sh":  "#!/bin/sh\nprintf fixture-installer\\n\n",
		"install.cmd": "@echo fixture-installer\r\n",
	}
	if err := os.WriteFile(installSh, []byte(installerContents["install.sh"]), 0o755); err != nil {
		t.Fatalf("write install.sh fixture: %v", err)
	}
	if err := os.WriteFile(installCmd, []byte(installerContents["install.cmd"]), 0o644); err != nil {
		t.Fatalf("write install.cmd fixture: %v", err)
	}
	for name, contents := range installerContents {
		archiveContents[name] = contents
	}
	changelog := filepath.Join(workDir, "v0.1.0.md")
	if err := os.WriteFile(changelog, []byte("# v0.1.0 — 2026-07-12\n\n## Added\n\n- Public change.\n"), 0o644); err != nil {
		t.Fatalf("write changelog fixture: %v", err)
	}
	changelogChinese := filepath.Join(workDir, "v0.1.0.zh-CN.md")
	if err := os.WriteFile(changelogChinese, []byte("# v0.1.0 — 2026-07-12\n\n## 新增\n\n- 公开变更。\n"), 0o644); err != nil {
		t.Fatalf("write Simplified Chinese changelog fixture: %v", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate fixture signing key: %v", err)
	}
	privateKeyPath := filepath.Join(workDir, "signing-key.pem")
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("encode fixture signing key: %v", err)
	}
	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER}), 0o600); err != nil {
		t.Fatalf("write fixture signing key: %v", err)
	}
	outputDir := filepath.Join(workDir, "release")
	if err := run([]string{
		"finalize",
		"--version", version,
		"--input-dir", inputDir,
		"--output-dir", outputDir,
		"--changelog", changelog,
		"--changelog-zh", changelogChinese,
		"--install-sh", installSh,
		"--install-cmd", installCmd,
		"--private-key", privateKeyPath,
		"--key-id", "fixture-2026",
	}); err != nil {
		t.Fatalf("finalize release assets: %v", err)
	}

	checksums, err := os.ReadFile(filepath.Join(outputDir, "checksums.txt"))
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	wantChecksums := expectedChecksums(archiveContents)
	if string(checksums) != wantChecksums {
		t.Fatalf("checksums content mismatch\nwant:\n%s\ngot:\n%s", wantChecksums, checksums)
	}
	var manifest struct {
		KeyID           string `json:"key_id"`
		ChecksumsSHA256 string `json:"checksums_sha256"`
		Signature       string `json:"signature"`
	}
	manifestBody, err := os.ReadFile(filepath.Join(outputDir, "checksums.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.KeyID != "fixture-2026" {
		t.Fatalf("key ID = %q, want fixture-2026", manifest.KeyID)
	}
	sum := sha256.Sum256(checksums)
	if manifest.ChecksumsSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("checksums SHA-256 = %q, want %x", manifest.ChecksumsSHA256, sum)
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil {
		t.Fatalf("decode manifest signature: %v", err)
	}
	if !ed25519.Verify(publicKey, checksums, signature) {
		t.Fatal("manifest signature does not verify the raw checksums bytes")
	}
	releaseNotes, err := os.ReadFile(filepath.Join(outputDir, "release-notes.md"))
	if err != nil {
		t.Fatalf("read release notes: %v", err)
	}
	if string(releaseNotes) != "# English\n\n## Added\n\n- Public change.\n\n---\n\n# 简体中文\n\n## 新增\n\n- 公开变更。\n" {
		t.Fatalf("release notes = %q", releaseNotes)
	}
	for name, contents := range archiveContents {
		body, err := os.ReadFile(filepath.Join(outputDir, name))
		if err != nil {
			t.Fatalf("read copied archive %s: %v", name, err)
		}
		if string(body) != contents {
			t.Fatalf("copied archive %s = %q, want %q", name, body, contents)
		}
	}
}

func TestFinalizeRejectsMissingOrDuplicateChangelogSectionWithoutPublishing(t *testing.T) {
	t.Parallel()

	for name, changelogBody := range map[string]string{
		"missing":   "# v0.2.0 — 2026-07-12\n\n## Added\n\nPending.\n",
		"duplicate": "# v0.1.0 — 2026-07-12\n\nFirst.\n\n# v0.1.0 — 2026-07-12\n\nSecond.\n",
	} {
		t.Run(name, func(t *testing.T) {
			workDir := newTestWorkDir(t)
			inputDir := filepath.Join(workDir, "input")
			if err := os.Mkdir(inputDir, 0o755); err != nil {
				t.Fatalf("create input directory: %v", err)
			}
			writeArchiveFixtures(t, inputDir, "0.1.0")
			changelog := filepath.Join(workDir, "v0.1.0.md")
			if err := os.WriteFile(changelog, []byte(changelogBody), 0o644); err != nil {
				t.Fatalf("write changelog fixture: %v", err)
			}
			changelogChinese := filepath.Join(workDir, "v0.1.0.zh-CN.md")
			if err := os.WriteFile(changelogChinese, []byte("# v0.1.0 — 2026-07-12\n\n## 新增\n\n- 公开变更。\n"), 0o644); err != nil {
				t.Fatalf("write Simplified Chinese changelog fixture: %v", err)
			}
			privateKeyPath := writeFixturePrivateKey(t, workDir)
			outputDir := filepath.Join(workDir, "release")

			err := run([]string{
				"finalize",
				"--version", "0.1.0",
				"--input-dir", inputDir,
				"--output-dir", outputDir,
				"--changelog", changelog,
				"--changelog-zh", changelogChinese,
				"--private-key", privateKeyPath,
				"--key-id", "fixture-2026",
			})
			if err == nil || !strings.Contains(err.Error(), "changelog") {
				t.Fatalf("finalize error = %v, want changelog rejection", err)
			}
			if _, statErr := os.Lstat(outputDir); !os.IsNotExist(statErr) {
				t.Fatalf("failed finalize published output: %v", statErr)
			}
		})
	}
}

func TestFinalizeRequiresSimplifiedChineseChangelog(t *testing.T) {
	t.Parallel()

	workDir := newTestWorkDir(t)
	inputDir := filepath.Join(workDir, "input")
	if err := os.Mkdir(inputDir, 0o755); err != nil {
		t.Fatalf("create input directory: %v", err)
	}
	changelog := filepath.Join(workDir, "v0.1.0.md")
	if err := os.WriteFile(changelog, []byte("# v0.1.0 — 2026-07-12\n\n## Added\n\n- Public change.\n"), 0o644); err != nil {
		t.Fatalf("write changelog fixture: %v", err)
	}

	err := run([]string{
		"finalize",
		"--version", "0.1.0",
		"--input-dir", inputDir,
		"--output-dir", filepath.Join(workDir, "release"),
		"--changelog", changelog,
		"--key-id", "fixture-2026",
	})
	if err == nil || !strings.Contains(err.Error(), "Simplified Chinese changelog") {
		t.Fatalf("finalize error = %v, want missing Simplified Chinese changelog rejection", err)
	}
}

func newTestWorkDir(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp(findTestRepositoryRoot(t), ".pixiv-releaseassets-test.")
	if err != nil {
		t.Fatalf("create repository-local temporary directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove temporary directory: %v", err)
		}
	})
	return directory
}

func writeArchiveFixtures(t *testing.T, directory, version string) map[string]string {
	t.Helper()
	contents := make(map[string]string, len(fixedTargets))
	for _, target := range fixedTargets {
		name := archiveName(version, target)
		body := "archive " + name
		if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write archive fixture %s: %v", name, err)
		}
		contents[name] = body
	}
	return contents
}

func writeFixturePrivateKey(t *testing.T, directory string) string {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate fixture signing key: %v", err)
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("encode fixture signing key: %v", err)
	}
	path := filepath.Join(directory, "signing-key.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER}), 0o600); err != nil {
		t.Fatalf("write fixture signing key: %v", err)
	}
	return path
}

func expectedChecksums(archives map[string]string) string {
	names := make([]string, 0, len(archives))
	for name := range archives {
		names = append(names, name)
	}
	sort.Strings(names)
	var result string
	for _, name := range names {
		sum := sha256.Sum256([]byte(archives[name]))
		result += hex.EncodeToString(sum[:]) + "  " + name + "\n"
	}
	return result
}

func findTestRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && info.Mode().IsRegular() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find repository root")
		}
		directory = parent
	}
}
