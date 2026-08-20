package nativeevidence

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira/staticlib"
)

func recordEvidence(options recordOptions) (evidenceRecord, error) {
	if !semanticVersionPattern.MatchString(options.version) {
		return evidenceRecord{}, fmt.Errorf("version is not semantic: %q", options.version)
	}
	if !gitCommitPattern.MatchString(options.sourceCommit) {
		return evidenceRecord{}, fmt.Errorf("source commit must be a 40-character lowercase Git SHA: %q", options.sourceCommit)
	}
	target, ok := nativeTargets[options.target]
	if !ok || target.rustTarget != options.rustTarget {
		return evidenceRecord{}, fmt.Errorf("unsupported target binding %q / %q", options.target, options.rustTarget)
	}
	if err := requireRegularFile(options.staticlib, "staticlib"); err != nil {
		return evidenceRecord{}, err
	}
	if filepath.Base(options.staticlib) != target.staticlib {
		return evidenceRecord{}, fmt.Errorf("staticlib name %q does not match %s", filepath.Base(options.staticlib), target.staticlib)
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
	root, err := requireDirectory(options.repoRoot, "repository root")
	if err != nil {
		return evidenceRecord{}, err
	}
	sourceDigest, err := staticlib.CalculateRustSourceDigest(
		filepath.Join(root, "internal", "media", "ugoira", "rust"),
		filepath.Join(root, "third_party", "rust", "quantette-0.6.0"),
	)
	if err != nil {
		return evidenceRecord{}, fmt.Errorf("calculate Rust source digest: %w", err)
	}
	staticlibDigest, err := fileSHA256(options.staticlib)
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
	members, err := verifyArchive(root, options.archive, filepath.Base(options.binary), binaryDigest)
	if err != nil {
		return evidenceRecord{}, err
	}
	archiveDigest, err := fileSHA256(options.archive)
	if err != nil {
		return evidenceRecord{}, err
	}
	record := evidenceRecord{
		Schema:       evidenceSchemaVersion,
		Target:       evidenceTarget{GOOS: target.goos, GOARCH: target.goarch, RustTarget: target.rustTarget},
		SourceCommit: options.sourceCommit,
		SourceDigest: sourceDigest,
		Staticlib:    evidenceFile{Name: filepath.Base(options.staticlib), SHA256: staticlibDigest},
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
