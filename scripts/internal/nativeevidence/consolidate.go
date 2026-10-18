package nativeevidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// consolidateEvidence 只接受六个完整且同源的 runner artifact 目录；它先在全新 sibling
// 目录验证和组装，再一次 rename 发布。已有输出、缺目标、hash 变化或 symlink 一律失败，
// 只发布同一版本、同一源码 commit 的完整 Go binary/archive 证据集合。
func consolidateEvidence(options consolidateOptions) error {
	if !strings.HasPrefix(options.expectedVersion, "v") || !semanticVersionPattern.MatchString(strings.TrimPrefix(options.expectedVersion, "v")) {
		return fmt.Errorf("expected version is not a v-prefixed semantic version: %q", options.expectedVersion)
	}
	if !gitCommitPattern.MatchString(options.expectedCommit) {
		return fmt.Errorf("expected commit must be a 40-character lowercase Git SHA: %q", options.expectedCommit)
	}
	_, err := requireSecureDirectory(options.repoRoot, "repository root")
	if err != nil {
		return err
	}
	inputDir, err := requireSecureDirectory(options.inputDir, "input directory")
	if err != nil {
		return err
	}
	if err := requireNewOutput(options.outputDir); err != nil {
		return err
	}
	locations, err := readEvidenceLocations(inputDir)
	if err != nil {
		return err
	}
	if len(locations) != len(nativeTargets) {
		return fmt.Errorf("native evidence must contain exactly six target records, got %d", len(locations))
	}
	if err := verifyEvidenceLocations(locations, options.expectedVersion, options.expectedCommit); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(options.outputDir), ".native-evidence-consolidate-")
	if err != nil {
		return fmt.Errorf("create evidence staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	sort.Slice(locations, func(i, j int) bool {
		left, right := locations[i].record.Target, locations[j].record.Target
		return left.GOOS+"/"+left.GOARCH < right.GOOS+"/"+right.GOARCH
	})
	summary := evidenceSummary{Schema: evidenceSchemaVersion, SourceCommit: options.expectedCommit, Version: options.expectedVersion}
	for _, location := range locations {
		record := location.record
		platform := record.Target.GOOS + "-" + record.Target.GOARCH
		destination := filepath.Join(stage, platform)
		if err := os.Mkdir(destination, 0o755); err != nil {
			return fmt.Errorf("create evidence destination for %s: %w", platform, err)
		}
		for _, artifact := range []evidenceFile{record.Binary.evidenceFile, record.Archive.evidenceFile} {
			if err := copyVerifiedFile(filepath.Join(location.dir, artifact.Name), filepath.Join(destination, artifact.Name), artifact.SHA256); err != nil {
				return fmt.Errorf("copy verified artifact for %s: %w", platform, err)
			}
		}
		if err := writeNewJSON(filepath.Join(destination, "native-evidence.json"), record); err != nil {
			return err
		}
		summary.Records = append(summary.Records, record)
	}
	if err := writeNewJSON(filepath.Join(stage, "summary.json"), summary); err != nil {
		return err
	}
	if err := os.Rename(stage, options.outputDir); err != nil {
		return fmt.Errorf("publish complete Go evidence: %w", err)
	}
	return nil
}

func readEvidenceLocations(inputDir string) ([]evidenceLocation, error) {
	var locations []evidenceLocation
	err := filepath.WalkDir(inputDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("evidence input contains a symlink: %s", path)
		}
		if entry.IsDir() || entry.Name() != "native-evidence.json" {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("evidence record is not a regular file: %s", path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.DisallowUnknownFields()
		var record evidenceRecord
		if err := decoder.Decode(&record); err != nil {
			return fmt.Errorf("decode evidence record %s: %w", path, err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return fmt.Errorf("evidence record %s contains trailing data", path)
		}
		locations = append(locations, evidenceLocation{record: record, dir: filepath.Dir(path)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read evidence artifacts: %w", err)
	}
	return locations, nil
}

func verifyEvidenceLocations(locations []evidenceLocation, expectedVersion, expectedCommit string) error {
	seen := make(map[string]struct{}, len(locations))
	for _, location := range locations {
		record := location.record
		if record.Schema != evidenceSchemaVersion {
			return fmt.Errorf("evidence record for %s/%s uses unsupported schema %d", record.Target.GOOS, record.Target.GOARCH, record.Schema)
		}
		if !gitCommitPattern.MatchString(record.SourceCommit) {
			return fmt.Errorf("source metadata for %s is incomplete", record.Target.GOOS+"/"+record.Target.GOARCH)
		}
		if record.SourceCommit != expectedCommit {
			return fmt.Errorf("source metadata for %s does not match the expected workflow run", record.Target.GOOS+"/"+record.Target.GOARCH)
		}
		platform := record.Target.GOOS + "/" + record.Target.GOARCH
		target, ok := nativeTargets[platform]
		if !ok {
			return fmt.Errorf("evidence record has an unsupported target binding %q", platform)
		}
		if _, duplicate := seen[platform]; duplicate {
			return fmt.Errorf("native evidence contains duplicate target %q", platform)
		}
		seen[platform] = struct{}{}
		binaryName := "pixiv"
		if target.goos == "windows" {
			binaryName = "pixiv.exe"
		}
		if err := verifyRecordedArtifact(location.dir, record.Binary.evidenceFile, binaryName); err != nil {
			return fmt.Errorf("validate binary for %s: %w", platform, err)
		}
		if !strings.HasPrefix(record.Binary.Version, "v") || !semanticVersionPattern.MatchString(strings.TrimPrefix(record.Binary.Version, "v")) {
			return fmt.Errorf("binary version for %s is invalid", platform)
		}
		if record.Binary.Version != expectedVersion {
			return fmt.Errorf("binary version for %s does not match the expected workflow run", platform)
		}
		if err := verifyRecordedArtifact(location.dir, record.Archive.evidenceFile, expectedArchiveName(strings.TrimPrefix(record.Binary.Version, "v"), target)); err != nil {
			return fmt.Errorf("validate archive for %s: %w", platform, err)
		}
		if err := validateRecordedMembers(record.Archive.Members); err != nil {
			return fmt.Errorf("validate archive members for %s: %w", platform, err)
		}
		if err := verifyRecordedArchiveMembers(filepath.Join(location.dir, record.Archive.Name), record.Archive.Members); err != nil {
			return fmt.Errorf("verify archive members for %s: %w", platform, err)
		}
		expected := map[string]string{record.Binary.Name: record.Binary.SHA256}
		if err := requireExactEvidenceMembers(record.Archive.Members, expected); err != nil {
			return fmt.Errorf("archive members for %s do not match the audited tree: %w", platform, err)
		}
	}
	if len(seen) != len(nativeTargets) {
		return errors.New("native evidence must contain all six target records")
	}
	return nil
}

func verifyRecordedArtifact(directory string, artifact evidenceFile, expectedName string) error {
	if artifact.Name != expectedName || !isSHA256(artifact.SHA256) || filepath.Base(artifact.Name) != artifact.Name {
		return fmt.Errorf("artifact metadata does not bind expected %q", expectedName)
	}
	path := filepath.Join(directory, artifact.Name)
	if err := requireRegularFile(path, "artifact"); err != nil {
		return err
	}
	digest, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if digest != artifact.SHA256 {
		return fmt.Errorf("artifact SHA-256 %q does not match recorded %q", digest, artifact.SHA256)
	}
	return nil
}

func validateRecordedMembers(members []evidenceFile) error {
	if len(members) == 0 {
		return errors.New("archive record has no regular members")
	}
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member.Name == "" || filepath.ToSlash(filepath.Clean(member.Name)) != member.Name || strings.HasPrefix(member.Name, "/") || strings.Contains(member.Name, "../") || strings.Contains(member.Name, "\\") || !isSHA256(member.SHA256) {
			return errors.New("archive member metadata is invalid")
		}
		if _, duplicate := seen[member.Name]; duplicate {
			return fmt.Errorf("archive member %q is duplicated", member.Name)
		}
		seen[member.Name] = struct{}{}
	}
	return nil
}

func requireExactEvidenceMembers(recorded []evidenceFile, expected map[string]string) error {
	if len(recorded) != len(expected) {
		return errors.New("member count differs")
	}
	for _, member := range recorded {
		if expected[member.Name] != member.SHA256 {
			return fmt.Errorf("member %q differs", member.Name)
		}
	}
	return nil
}

func verifyRecordedArchiveMembers(path string, recorded []evidenceFile) error {
	var (
		actual map[string]string
		err    error
	)
	if strings.HasSuffix(path, ".tar.gz") {
		actual, err = readTarGzMembers(path)
	} else if strings.HasSuffix(path, ".zip") {
		actual, err = readZIPMembers(path)
	} else {
		return errors.New("archive has an unsupported extension")
	}
	if err != nil {
		return err
	}
	if len(actual) != len(recorded) {
		return errors.New("archive members differ from the recorded evidence")
	}
	for _, member := range recorded {
		if actual[member.Name] != member.SHA256 {
			return fmt.Errorf("archive member %q differs from the recorded evidence", member.Name)
		}
	}
	return nil
}

func copyVerifiedFile(source, destination, expectedDigest string) error {
	if err := requireRegularFile(source, "source artifact"); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	failed := true
	defer func() {
		if failed {
			_ = os.Remove(destination)
		}
	}()
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(output, hasher), input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != expectedDigest {
		return fmt.Errorf("copied artifact SHA-256 %q does not match %q", got, expectedDigest)
	}
	failed = false
	return nil
}
