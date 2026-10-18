package nativeevidence

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
)

func recordEvidence(options recordOptions) (evidenceRecord, error) {
	if !semanticVersionPattern.MatchString(options.version) {
		return evidenceRecord{}, fmt.Errorf("version is not semantic: %q", options.version)
	}
	if !gitCommitPattern.MatchString(options.sourceCommit) {
		return evidenceRecord{}, fmt.Errorf("source commit must be a 40-character lowercase Git SHA: %q", options.sourceCommit)
	}
	target, ok := nativeTargets[options.target]
	if !ok {
		return evidenceRecord{}, fmt.Errorf("unsupported target %q", options.target)
	}
	binaryName := "pixiv"
	if target.goos == "windows" {
		binaryName = "pixiv.exe"
	}
	if filepath.Base(options.binary) != binaryName {
		return evidenceRecord{}, fmt.Errorf("binary name must be %q for target %s", binaryName, options.target)
	}
	if err := requireRegularFile(options.binary, "binary"); err != nil {
		return evidenceRecord{}, err
	}
	if err := requireRegularFile(options.archive, "archive"); err != nil {
		return evidenceRecord{}, err
	}
	if err := requireNewOutput(options.output); err != nil {
		return evidenceRecord{}, err
	}
	if filepath.Base(options.archive) != expectedArchiveName(options.version, target) {
		return evidenceRecord{}, fmt.Errorf("archive name %q does not match target %s", filepath.Base(options.archive), options.target)
	}
	_, err := requireDirectory(options.repoRoot, "repository root")
	if err != nil {
		return evidenceRecord{}, err
	}
	binaryDigest, err := fileSHA256(options.binary)
	if err != nil {
		return evidenceRecord{}, err
	}
	binaryVersion, err := readBinaryVersion(options.binary, "v"+options.version)
	if err != nil {
		return evidenceRecord{}, err
	}
	members, err := verifyArchive(options.archive, filepath.Base(options.binary), binaryDigest)
	if err != nil {
		return evidenceRecord{}, err
	}
	archiveDigest, err := fileSHA256(options.archive)
	if err != nil {
		return evidenceRecord{}, err
	}
	record := evidenceRecord{
		Schema:       evidenceSchemaVersion,
		Target:       evidenceTarget{GOOS: target.goos, GOARCH: target.goarch},
		SourceCommit: options.sourceCommit,
		Binary: evidenceBinary{
			evidenceFile: evidenceFile{Name: filepath.Base(options.binary), SHA256: binaryDigest},
			Version:      binaryVersion,
		},
		Archive: evidenceArchive{evidenceFile: evidenceFile{Name: filepath.Base(options.archive), SHA256: archiveDigest}, Members: members},
	}
	if err := writeNewJSON(options.output, record); err != nil {
		return evidenceRecord{}, err
	}
	return record, nil
}

func expectedArchiveName(version string, target nativeTarget) string {
	extension := ".tar.gz"
	if target.goos == "windows" {
		extension = ".zip"
	}
	return "pixiv-cli_" + version + "_" + target.goos + "_" + target.goarch + extension
}

func readBinaryVersion(binary, expectedVersion string) (string, error) {
	command := exec.Command(binary, "--version")
	body, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("run binary --version: %w", err)
	}
	// 发布预检要求完整单行逐字一致，避免接受尾随数据或旧 JSON 契约。
	expected := []byte("pixiv " + expectedVersion + "\n")
	if !bytes.Equal(body, expected) {
		return "", fmt.Errorf("binary --version output does not match %q", string(expected))
	}
	return expectedVersion, nil
}
