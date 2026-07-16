package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/download/staticlib"
)

func recordEvidence(options recordOptions) (evidenceRecord, error) {
	if !semanticVersionPattern.MatchString(options.version) {
		return evidenceRecord{}, fmt.Errorf("version is not semantic: %q", options.version)
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
		filepath.Join(root, "internal", "download", "ugoira_rs"),
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
	binaryInfo, err := readBinaryVersion(options.binary, "v"+options.version)
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
		Schema:       1,
		Target:       evidenceTarget{GOOS: target.goos, GOARCH: target.goarch, RustTarget: target.rustTarget},
		SourceDigest: sourceDigest,
		Staticlib:    evidenceFile{Name: filepath.Base(options.staticlib), SHA256: staticlibDigest},
		Binary: evidenceBinary{
			evidenceFile: evidenceFile{Name: filepath.Base(options.binary), SHA256: binaryDigest},
			Version:      binaryInfo.Version,
			Commit:       binaryInfo.Commit,
			BuildDate:    binaryInfo.BuildDate,
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

func readBinaryVersion(binary, expectedVersion string) (buildinfo.Info, error) {
	command := exec.Command(binary, "version", "--json")
	body, err := command.Output()
	if err != nil {
		return buildinfo.Info{}, fmt.Errorf("run binary version --json: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var info buildinfo.Info
	if err := decoder.Decode(&info); err != nil {
		return buildinfo.Info{}, fmt.Errorf("decode binary version --json: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return buildinfo.Info{}, errors.New("binary version --json contains trailing data")
	}
	if info.Version != expectedVersion || strings.TrimSpace(info.Commit) == "" || strings.TrimSpace(info.BuildDate) == "" {
		return buildinfo.Info{}, fmt.Errorf("binary version metadata does not match %q", expectedVersion)
	}
	return info, nil
}
