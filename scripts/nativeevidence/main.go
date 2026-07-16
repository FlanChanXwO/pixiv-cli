// Command nativeevidence records and validates non-release native runner evidence.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/download/staticlib"
	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

var actionReferencePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)

const canonicalCheckoutAction = "actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5"
const canonicalSetupGoAction = "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff"
const canonicalUploadArtifactAction = "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02"

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
var gitCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type nativeTarget struct {
	goos       string
	goarch     string
	rustTarget string
	staticlib  string
}

var nativeTargets = map[string]nativeTarget{
	"darwin/amd64":  {goos: "darwin", goarch: "amd64", rustTarget: "x86_64-apple-darwin", staticlib: "libugoira_rs.a"},
	"darwin/arm64":  {goos: "darwin", goarch: "arm64", rustTarget: "aarch64-apple-darwin", staticlib: "libugoira_rs.a"},
	"linux/amd64":   {goos: "linux", goarch: "amd64", rustTarget: "x86_64-unknown-linux-gnu", staticlib: "libugoira_rs.a"},
	"linux/arm64":   {goos: "linux", goarch: "arm64", rustTarget: "aarch64-unknown-linux-gnu", staticlib: "libugoira_rs.a"},
	"windows/amd64": {goos: "windows", goarch: "amd64", rustTarget: "x86_64-pc-windows-msvc", staticlib: "ugoira_rs.lib"},
	"windows/arm64": {goos: "windows", goarch: "arm64", rustTarget: "aarch64-pc-windows-msvc", staticlib: "ugoira_rs.lib"},
}

type recordOptions struct {
	repoRoot   string
	version    string
	target     string
	rustTarget string
	staticlib  string
	binary     string
	archive    string
	output     string
}

// evidenceRecord 是单个真实 runner 输出的可迁移审计记录。测试可构造 fixture 验证格式，
// 但只有 workflow 产生、绑定 main SHA 的记录才是 Task 33 所要求的 native evidence。
type evidenceRecord struct {
	Schema       int             `json:"schema"`
	Target       evidenceTarget  `json:"target"`
	SourceDigest string          `json:"source_digest"`
	Staticlib    evidenceFile    `json:"staticlib"`
	Binary       evidenceBinary  `json:"binary"`
	Archive      evidenceArchive `json:"archive"`
}

type evidenceTarget struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	RustTarget string `json:"rust_target"`
}

type evidenceFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type evidenceBinary struct {
	evidenceFile
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

type evidenceArchive struct {
	evidenceFile
	Members []evidenceFile `json:"members"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "native evidence: %v\n", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("usage: nativeevidence policy|record|consolidate")
	}
	switch arguments[0] {
	case "policy":
		return runPolicy(arguments[1:])
	case "record":
		return runRecord(arguments[1:])
	case "consolidate":
		return runConsolidate(arguments[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", arguments[0])
	}
}

func runPolicy(arguments []string) error {
	if len(arguments) != 2 || arguments[0] != "--workflow" {
		return errors.New("usage: nativeevidence policy --workflow PATH")
	}
	body, err := os.ReadFile(arguments[1])
	if err != nil {
		return fmt.Errorf("read workflow: %w", err)
	}
	return checkWorkflow(body)
}

func runRecord(arguments []string) error {
	flags := flag.NewFlagSet("record", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := recordOptions{}
	flags.StringVar(&options.repoRoot, "repo-root", "", "repository root")
	flags.StringVar(&options.version, "version", "", "semantic version without v")
	flags.StringVar(&options.target, "target", "", "GOOS/GOARCH target")
	flags.StringVar(&options.rustTarget, "rust-target", "", "Rust target triple")
	flags.StringVar(&options.staticlib, "staticlib", "", "native Rust static library")
	flags.StringVar(&options.binary, "binary", "", "versioned pixiv binary")
	flags.StringVar(&options.archive, "archive", "", "release-style archive")
	flags.StringVar(&options.output, "output", "", "new evidence JSON path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("record accepts no positional arguments: %q", flags.Arg(0))
	}
	_, err := recordEvidence(options)
	return err
}

func runConsolidate(arguments []string) error {
	flags := flag.NewFlagSet("consolidate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := consolidateOptions{}
	flags.StringVar(&options.repoRoot, "repo-root", "", "repository root matching the audited evidence source")
	flags.StringVar(&options.expectedVersion, "expected-version", "", "exact v-prefixed binary version from the audited main workflow run")
	flags.StringVar(&options.expectedCommit, "expected-commit", "", "exact main commit SHA from the audited workflow run")
	flags.StringVar(&options.inputDir, "input-dir", "", "directory containing downloaded native evidence artifacts")
	flags.StringVar(&options.outputDir, "output-dir", "", "new staticlib directory receiving a complete manifest")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("consolidate accepts no positional arguments: %q", flags.Arg(0))
	}
	return consolidateEvidence(options)
}

type consolidateOptions struct {
	repoRoot        string
	expectedVersion string
	expectedCommit  string
	inputDir        string
	outputDir       string
}

type evidenceLocation struct {
	record evidenceRecord
	dir    string
}

// consolidateEvidence 只接受六个完整且同源的 runner artifact 目录；它先在全新 sibling
// 目录验证和组装，再一次 rename 发布。已有输出、缺目标、hash 变化或 symlink 一律失败，
// 因而不会把部分或混代库伪装成 Task 13 的可提交 staticlib 集合。
func consolidateEvidence(options consolidateOptions) error {
	if !strings.HasPrefix(options.expectedVersion, "v") || !semanticVersionPattern.MatchString(strings.TrimPrefix(options.expectedVersion, "v")) {
		return fmt.Errorf("expected version is not a v-prefixed semantic version: %q", options.expectedVersion)
	}
	if !gitCommitPattern.MatchString(options.expectedCommit) {
		return fmt.Errorf("expected commit must be a 40-character lowercase Git SHA: %q", options.expectedCommit)
	}
	repoRoot, err := requireSecureDirectory(options.repoRoot, "repository root")
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
	sourceDigest, err := calculateSourceDigest(repoRoot)
	if err != nil {
		return err
	}
	if err := verifyEvidenceLocations(locations, repoRoot, sourceDigest, options.expectedVersion, options.expectedCommit); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(options.outputDir), ".native-evidence-consolidate-")
	if err != nil {
		return fmt.Errorf("create staticlib staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	manifest := staticlib.Manifest{Schema: 1, SourceDigest: sourceDigest, Artifacts: make(map[string]staticlib.ManifestAsset, len(locations))}
	for _, location := range locations {
		platform := location.record.Target.GOOS + "/" + location.record.Target.GOARCH
		target := nativeTargets[platform]
		source := filepath.Join(location.dir, location.record.Staticlib.Name)
		destinationDir := filepath.Join(stage, target.rustTarget)
		if err := os.MkdirAll(destinationDir, 0o755); err != nil {
			return fmt.Errorf("create staticlib destination for %s: %w", platform, err)
		}
		destination := filepath.Join(destinationDir, target.staticlib)
		if err := copyVerifiedFile(source, destination, location.record.Staticlib.SHA256); err != nil {
			return fmt.Errorf("copy staticlib for %s: %w", platform, err)
		}
		manifest.Artifacts[platform] = staticlib.ManifestAsset{
			Target: target.rustTarget,
			Path:   filepath.ToSlash(filepath.Join(target.rustTarget, target.staticlib)),
			SHA256: location.record.Staticlib.SHA256,
		}
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode staticlib manifest: %w", err)
	}
	if err := staticlib.ValidateManifestFiles(stage, body, sourceDigest); err != nil {
		return fmt.Errorf("validate staged staticlib manifest: %w", err)
	}
	if err := writeFreshFile(filepath.Join(stage, "manifest.json"), append(body, '\n')); err != nil {
		return fmt.Errorf("write staticlib manifest: %w", err)
	}
	if err := os.Rename(stage, options.outputDir); err != nil {
		return fmt.Errorf("publish complete staticlib evidence: %w", err)
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

func verifyEvidenceLocations(locations []evidenceLocation, repoRoot, sourceDigest, expectedVersion, expectedCommit string) error {
	seen := make(map[string]struct{}, len(locations))
	for _, location := range locations {
		record := location.record
		if record.Schema != 1 || record.SourceDigest != sourceDigest {
			return errors.New("evidence record source digest does not match the audited repository")
		}
		platform := record.Target.GOOS + "/" + record.Target.GOARCH
		target, ok := nativeTargets[platform]
		if !ok || target.rustTarget != record.Target.RustTarget {
			return fmt.Errorf("evidence record has an unsupported target binding %q", platform)
		}
		if _, duplicate := seen[platform]; duplicate {
			return fmt.Errorf("native evidence contains duplicate target %q", platform)
		}
		seen[platform] = struct{}{}
		if err := verifyRecordedArtifact(location.dir, record.Staticlib, target.staticlib); err != nil {
			return fmt.Errorf("validate staticlib for %s: %w", platform, err)
		}
		binaryName := "pixiv"
		if target.goos == "windows" {
			binaryName = "pixiv.exe"
		}
		if err := verifyRecordedArtifact(location.dir, record.Binary.evidenceFile, binaryName); err != nil {
			return fmt.Errorf("validate binary for %s: %w", platform, err)
		}
		if !strings.HasPrefix(record.Binary.Version, "v") || !semanticVersionPattern.MatchString(strings.TrimPrefix(record.Binary.Version, "v")) || strings.TrimSpace(record.Binary.Commit) == "" || strings.TrimSpace(record.Binary.BuildDate) == "" {
			return fmt.Errorf("binary metadata for %s is incomplete", platform)
		}
		if record.Binary.Version != expectedVersion || record.Binary.Commit != expectedCommit {
			return fmt.Errorf("binary metadata for %s does not match the expected workflow run", platform)
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
		expected, err := expectedArchiveMembers(repoRoot, record.Binary.Name, record.Binary.SHA256)
		if err != nil {
			return fmt.Errorf("calculate expected archive members for %s: %w", platform, err)
		}
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
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
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

func writeFreshFile(path string, body []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

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

func requireDirectory(path, label string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s must be a directory", label)
	}
	return filepath.Clean(path), nil
}

func requireSecureDirectory(path, label string) (string, error) {
	directory, err := requireDirectory(path, label)
	if err != nil {
		return "", err
	}
	for current := directory; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("inspect %s ancestor: %w", label, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%s contains a symlink ancestor: %s", label, current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return directory, nil
}

func calculateSourceDigest(repoRoot string) (string, error) {
	digest, err := staticlib.CalculateRustSourceDigest(
		filepath.Join(repoRoot, "internal", "download", "ugoira_rs"),
		filepath.Join(repoRoot, "third_party", "rust", "quantette-0.6.0"),
	)
	if err != nil {
		return "", fmt.Errorf("calculate Rust source digest: %w", err)
	}
	return digest, nil
}

func requireRegularFile(path, label string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s is required", label)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a non-symlink regular file", label)
	}
	return nil
}

func requireNewOutput(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("output is required")
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("output already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output: %w", err)
	}
	parent := filepath.Dir(path)
	if _, err := requireDirectory(parent, "output directory"); err != nil {
		return err
	}
	for directory := filepath.Clean(parent); ; directory = filepath.Dir(directory) {
		info, err := os.Lstat(directory)
		if err != nil {
			return fmt.Errorf("inspect output directory ancestor: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output directory contains a symlink ancestor: %s", directory)
		}
		parentDirectory := filepath.Dir(directory)
		if parentDirectory == directory {
			break
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	if err := requireRegularFile(path, "file"); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
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

func verifyArchive(repoRoot, archive, binaryName, binaryDigest string) ([]evidenceFile, error) {
	expected, err := expectedArchiveMembers(repoRoot, binaryName, binaryDigest)
	if err != nil {
		return nil, err
	}
	var actual map[string]string
	if strings.HasSuffix(archive, ".tar.gz") {
		actual, err = readTarGzMembers(archive)
	} else if strings.HasSuffix(archive, ".zip") {
		actual, err = readZIPMembers(archive)
	} else {
		return nil, errors.New("archive must end with .tar.gz or .zip")
	}
	if err != nil {
		return nil, err
	}
	if len(actual) != len(expected) {
		return nil, fmt.Errorf("archive regular members = %d, want complete set of %d", len(actual), len(expected))
	}
	members := make([]evidenceFile, 0, len(expected))
	for name, want := range expected {
		got, ok := actual[name]
		if !ok || got != want {
			return nil, fmt.Errorf("archive member %q does not match expected binary or license content", name)
		}
		members = append(members, evidenceFile{Name: name, SHA256: got})
	}
	sort.Slice(members, func(left, right int) bool { return members[left].Name < members[right].Name })
	return members, nil
}

func expectedArchiveMembers(repoRoot, binaryName, binaryDigest string) (map[string]string, error) {
	expected := map[string]string{binaryName: binaryDigest}
	for _, relative := range []string{"LICENSE", "THIRD_PARTY_LICENSES.md"} {
		digest, err := fileSHA256(filepath.Join(repoRoot, relative))
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", relative, err)
		}
		expected[relative] = digest
	}
	licenses := filepath.Join(repoRoot, "third_party", "licenses")
	err := filepath.WalkDir(licenses, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("license tree contains a symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("license tree contains a non-regular file: %s", path)
		}
		relative, err := filepath.Rel(licenses, path)
		if err != nil {
			return err
		}
		digest, err := fileSHA256(path)
		if err != nil {
			return err
		}
		expected[filepath.ToSlash(filepath.Join("third_party", "licenses", relative))] = digest
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read complete license tree: %w", err)
	}
	if len(expected) == 3 {
		return nil, errors.New("license tree contains no regular files")
	}
	return expected, nil
}

func readTarGzMembers(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("open tar.gz: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	members := make(map[string]string)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar member: %w", err)
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("archive contains a non-regular member %q", header.Name)
		}
		if err := addArchiveMember(members, header.Name, reader); err != nil {
			return nil, err
		}
	}
	return members, nil
}

func readZIPMembers(path string) (map[string]string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer reader.Close()
	members := make(map[string]string)
	for _, member := range reader.File {
		info := member.FileInfo()
		if info.IsDir() {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("archive contains a non-regular member %q", member.Name)
		}
		body, err := member.Open()
		if err != nil {
			return nil, fmt.Errorf("open zip member %q: %w", member.Name, err)
		}
		err = addArchiveMember(members, member.Name, body)
		closeErr := body.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close zip member %q: %w", member.Name, closeErr)
		}
	}
	return members, nil
}

func addArchiveMember(members map[string]string, name string, body io.Reader) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "../") || name == ".." {
		return fmt.Errorf("archive member path is unsafe: %q", name)
	}
	if _, duplicate := members[name]; duplicate {
		return fmt.Errorf("archive contains duplicate regular member %q", name)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, body); err != nil {
		return fmt.Errorf("hash archive member %q: %w", name, err)
	}
	members[name] = hex.EncodeToString(hasher.Sum(nil))
	return nil
}

func writeNewJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode evidence JSON: %w", err)
	}
	body = append(body, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".native-evidence-*.json")
	if err != nil {
		return fmt.Errorf("create evidence staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set evidence staging mode: %w", err)
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write evidence JSON: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close evidence JSON: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish evidence JSON: %w", err)
	}
	return nil
}

func findRepositoryRoot(t interface {
	Helper()
	Fatalf(string, ...any)
}) string {
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
			t.Fatalf("find repository root from %s", directory)
		}
		directory = parent
	}
}

// checkWorkflow 先拒绝发布凭据与副作用，再验证该入口只能从 main 或人工 dispatch 启动。
// 后续的 evidence 记录与回填同样依赖这个可在本地执行的 fail-closed policy。
func checkWorkflow(body []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}
	if err := workflowpolicy.RejectAmbiguousYAML(&document); err != nil {
		return err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return errors.New("workflow must contain exactly one mapping document")
	}
	root := document.Content[0]
	if err := requireOnlyMappingKeys(root, "name", "on", "permissions", "jobs"); err != nil {
		return errors.New("native evidence workflow root must contain only its audited fields")
	}
	if workflowpolicy.ContainsSecretReference(root) {
		return errors.New("native evidence workflow must not reference secrets")
	}
	if err := checkNativeEvidenceTrigger(root); err != nil {
		return err
	}
	if err := checkEmptyPermissions(root); err != nil {
		return err
	}
	jobs, ok := workflowpolicy.MappingValue(root, "jobs")
	if !ok || jobs.Kind != yaml.MappingNode || len(jobs.Content) != 2 {
		return errors.New("native evidence workflow must have exactly one job")
	}
	jobName, job := jobs.Content[0], jobs.Content[1]
	if jobName.Value != "native_evidence" || job.Kind != yaml.MappingNode {
		return errors.New("native evidence workflow must have a native_evidence job")
	}
	if err := checkActionReferences(root); err != nil {
		return err
	}
	if err := checkNoReleaseSideEffects(root); err != nil {
		return err
	}
	return checkNativeEvidenceJob(job)
}

func checkNativeEvidenceJob(job *yaml.Node) error {
	if err := requireOnlyMappingKeys(job, "name", "runs-on", "permissions", "strategy", "steps"); err != nil {
		return errors.New("native evidence job must contain only its audited fields")
	}
	if err := workflowpolicy.RequireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return errors.New("native evidence job must run only on its audited matrix runner")
	}
	if err := checkContentsReadPermission(job); err != nil {
		return err
	}
	strategy, ok := workflowpolicy.MappingValue(job, "strategy")
	if !ok || strategy.Kind != yaml.MappingNode || requireOnlyMappingKeys(strategy, "fail-fast", "matrix") != nil || workflowpolicy.RequireScalar(strategy, "fail-fast", "false") != nil {
		return errors.New("native evidence job must have a fail-fast false matrix strategy")
	}
	matrix, ok := workflowpolicy.MappingValue(strategy, "matrix")
	if !ok || matrix.Kind != yaml.MappingNode {
		return errors.New("native evidence job must have a matrix")
	}
	if err := checkNativeEvidenceMatrix(matrix); err != nil {
		return err
	}
	steps, ok := workflowpolicy.MappingValue(job, "steps")
	if !ok || steps.Kind != yaml.SequenceNode || len(steps.Content) != 12 {
		return errors.New("native evidence job must contain its complete audited step sequence")
	}
	if err := requireCanonicalCheckout(steps.Content[0]); err != nil {
		return err
	}
	if err := requireCanonicalSetupGo(steps.Content[1]); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[2], "Check native evidence workflow policy", "go run ./scripts/nativeevidence policy --workflow .github/workflows/native-evidence.yml"); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[3], "Require audited main ref", "test \"$GITHUB_REF\" = 'refs/heads/main'"); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[4], "Install the native Rust target", "rustup target add '${{ matrix.rust_target }}'"); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[5], "Check vendored Rust sources", "sh scripts/test-rust-vendor.sh"); err != nil {
		return err
	}
	if err := requireDirectRunStep(steps.Content[6], "Build the native Rust static library", "bash scripts/build-staticlibs.sh --target '${{ matrix.rust_target }}'"); err != nil {
		return err
	}
	if err := requireMultilineRunStep(steps.Content[7], "Run native GIF/APNG smoke", canonicalNativeSmokeRun); err != nil {
		return err
	}
	if err := requireMultilineRunStep(steps.Content[8], "Build and run the versioned native binary", canonicalBuildBinaryRun); err != nil {
		return err
	}
	if err := requireMultilineRunStep(steps.Content[9], "Package the versioned native binary", canonicalPackageRun); err != nil {
		return err
	}
	if err := requireMultilineRunStep(steps.Content[10], "Record staticlib, binary and archive evidence", canonicalRecordRun); err != nil {
		return err
	}
	if err := requireUploadEvidenceStep(steps.Content[11]); err != nil {
		return err
	}
	return nil
}

var nativeEvidenceMatrixTargets = map[string]struct{}{
	"macos-15-intel|darwin|amd64|x86_64-apple-darwin|darwin-amd64":       {},
	"macos-15|darwin|arm64|aarch64-apple-darwin|darwin-arm64":            {},
	"ubuntu-24.04|linux|amd64|x86_64-unknown-linux-gnu|linux-amd64":      {},
	"ubuntu-24.04-arm|linux|arm64|aarch64-unknown-linux-gnu|linux-arm64": {},
	"windows-2025|windows|amd64|x86_64-pc-windows-msvc|windows-amd64":    {},
	"windows-11-arm|windows|arm64|aarch64-pc-windows-msvc|windows-arm64": {},
}

func checkNativeEvidenceMatrix(matrix *yaml.Node) error {
	if requireOnlyMappingKeys(matrix, "include") != nil {
		return errors.New("native evidence matrix must contain only its six audited targets")
	}
	include, ok := workflowpolicy.MappingValue(matrix, "include")
	if !ok || include.Kind != yaml.SequenceNode || len(include.Content) != len(nativeEvidenceMatrixTargets) {
		return errors.New("native evidence matrix must contain exactly the six audited targets")
	}
	seen := make(map[string]struct{}, len(include.Content))
	for _, entry := range include.Content {
		if entry.Kind != yaml.MappingNode || requireOnlyMappingKeys(entry, "runner", "goos", "goarch", "rust_target", "artifact") != nil {
			return errors.New("native evidence matrix must contain exactly the six audited targets")
		}
		parts := make([]string, 0, 5)
		for _, key := range []string{"runner", "goos", "goarch", "rust_target", "artifact"} {
			value, ok := workflowpolicy.MappingValue(entry, key)
			if !ok || value.Kind != yaml.ScalarNode {
				return errors.New("native evidence matrix must contain exactly the six audited targets")
			}
			parts = append(parts, value.Value)
		}
		identity := strings.Join(parts, "|")
		if _, ok := nativeEvidenceMatrixTargets[identity]; !ok {
			return errors.New("native evidence matrix must contain exactly the six audited targets")
		}
		if _, duplicate := seen[identity]; duplicate {
			return errors.New("native evidence matrix must contain exactly the six audited targets")
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func requireCanonicalCheckout(step *yaml.Node) error {
	if requireOnlyMappingKeys(step, "uses", "with") != nil || workflowpolicy.RequireScalar(step, "uses", canonicalCheckoutAction) != nil {
		return errors.New("native evidence job must use the canonical credential-free checkout")
	}
	with, ok := workflowpolicy.MappingValue(step, "with")
	if !ok || requireOnlyMappingKeys(with, "persist-credentials") != nil || workflowpolicy.RequireScalar(with, "persist-credentials", "false") != nil {
		return errors.New("native evidence job must use the canonical credential-free checkout")
	}
	return nil
}

func requireCanonicalSetupGo(step *yaml.Node) error {
	if requireOnlyMappingKeys(step, "uses", "with") != nil || workflowpolicy.RequireScalar(step, "uses", canonicalSetupGoAction) != nil {
		return errors.New("native evidence job must use the canonical Go setup action")
	}
	with, ok := workflowpolicy.MappingValue(step, "with")
	if !ok || requireOnlyMappingKeys(with, "go-version") != nil || workflowpolicy.RequireScalar(with, "go-version", "1.26.3") != nil {
		return errors.New("native evidence job must use the canonical Go setup action")
	}
	return nil
}

func requireDirectRunStep(step *yaml.Node, name, command string) error {
	if requireOnlyMappingKeys(step, "name", "shell", "run") != nil || workflowpolicy.RequireScalar(step, "name", name) != nil || workflowpolicy.RequireScalar(step, "shell", "bash") != nil || workflowpolicy.RequireScalar(step, "run", command) != nil {
		return fmt.Errorf("native evidence job must retain direct step %q", name)
	}
	return nil
}

const canonicalNativeSmokeRun = `set -eu
if [ '${{ matrix.goos }}' = windows ]; then
  export CC='clang -fuse-ld=lld'
fi
go test ./internal/download -run '^TestRustUgoiraEncoderNativeGIFAndAPNG$' -count=1
`

const canonicalBuildBinaryRun = `set -eu
if [ '${{ matrix.goos }}' = windows ]; then
  export CC='clang -fuse-ld=lld'
fi
version="0.1.0-native-evidence.${GITHUB_RUN_ID}"
go run ./scripts/releaseassets validate --version "$version"
mkdir -p evidence
binary='evidence/pixiv'
if [ '${{ matrix.goos }}' = windows ]; then
  binary='evidence/pixiv.exe'
fi
go build -trimpath -buildvcs=false \
  -ldflags "-X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.Version=v${version} -X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.Commit=${GITHUB_SHA} -X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o "$binary" ./cmd/pixiv
"$binary" version --json
`

const canonicalPackageRun = `set -eu
version="0.1.0-native-evidence.${GITHUB_RUN_ID}"
binary='evidence/pixiv'
if [ '${{ matrix.goos }}' = windows ]; then
  binary='evidence/pixiv.exe'
fi
go run ./scripts/releaseassets package \
  --repo-root . \
  --version "$version" \
  --target '${{ matrix.goos }}/${{ matrix.goarch }}' \
  --binary "$binary" \
  --output-dir evidence
`

const canonicalRecordRun = `set -eu
version="0.1.0-native-evidence.${GITHUB_RUN_ID}"
staticlib='libugoira_rs.a'
if [ '${{ matrix.goos }}' = windows ]; then
  staticlib='ugoira_rs.lib'
fi
cp "internal/download/ugoira_rs/staticlib/${{ matrix.rust_target }}/$staticlib" "evidence/$staticlib"
archive="evidence/pixiv-cli_${version}_${{ matrix.goos }}_${{ matrix.goarch }}"
if [ '${{ matrix.goos }}' = windows ]; then
  archive="$archive.zip"
else
  archive="$archive.tar.gz"
fi
binary='evidence/pixiv'
if [ '${{ matrix.goos }}' = windows ]; then
  binary='evidence/pixiv.exe'
fi
go run ./scripts/nativeevidence record \
  --repo-root . \
  --version "$version" \
  --target '${{ matrix.goos }}/${{ matrix.goarch }}' \
  --rust-target '${{ matrix.rust_target }}' \
  --staticlib "evidence/$staticlib" \
  --binary "$binary" \
  --archive "$archive" \
  --output evidence/native-evidence.json
`

func requireMultilineRunStep(step *yaml.Node, name, canonical string) error {
	if requireOnlyMappingKeys(step, "name", "shell", "run") != nil || workflowpolicy.RequireScalar(step, "name", name) != nil || workflowpolicy.RequireScalar(step, "shell", "bash") != nil {
		return fmt.Errorf("native evidence job must retain step %q", name)
	}
	run, ok := workflowpolicy.MappingValue(step, "run")
	if !ok || run.Kind != yaml.ScalarNode || run.Value != canonical {
		return fmt.Errorf("native evidence job must retain guarded step %q", name)
	}
	return nil
}

func requireUploadEvidenceStep(step *yaml.Node) error {
	if requireOnlyMappingKeys(step, "name", "uses", "with") != nil || workflowpolicy.RequireScalar(step, "name", "Upload native evidence") != nil || workflowpolicy.RequireScalar(step, "uses", canonicalUploadArtifactAction) != nil {
		return errors.New("native evidence job must upload only its audited evidence artifact")
	}
	with, ok := workflowpolicy.MappingValue(step, "with")
	if !ok || requireOnlyMappingKeys(with, "name", "path", "if-no-files-found") != nil || workflowpolicy.RequireScalar(with, "name", "native-evidence-${{ matrix.artifact }}") != nil || workflowpolicy.RequireScalar(with, "path", "evidence") != nil || workflowpolicy.RequireScalar(with, "if-no-files-found", "error") != nil {
		return errors.New("native evidence job must upload only its audited evidence artifact")
	}
	return nil
}

func checkNativeEvidenceTrigger(root *yaml.Node) error {
	on, ok := workflowpolicy.MappingValue(root, "on")
	if !ok || on.Kind != yaml.MappingNode {
		return errors.New("native evidence workflow must have an on mapping")
	}
	push, hasPush := workflowpolicy.MappingValue(on, "push")
	dispatch, hasDispatch := workflowpolicy.MappingValue(on, "workflow_dispatch")
	if !hasPush || !hasDispatch || len(on.Content) != 4 || push.Kind != yaml.MappingNode || dispatch.Kind != yaml.MappingNode || len(dispatch.Content) != 0 {
		return errors.New("native evidence workflow must use only main push and workflow_dispatch triggers")
	}
	branches, ok := workflowpolicy.MappingValue(push, "branches")
	if !ok || len(push.Content) != 2 || branches.Kind != yaml.SequenceNode || len(branches.Content) != 1 || branches.Content[0].Value != "main" {
		return errors.New("native evidence workflow push trigger must be limited to main")
	}
	return nil
}

func checkEmptyPermissions(root *yaml.Node) error {
	permissions, ok := workflowpolicy.MappingValue(root, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode || len(permissions.Content) != 0 {
		return errors.New("native evidence workflow global permissions must be an empty mapping")
	}
	return nil
}

func checkContentsReadPermission(job *yaml.Node) error {
	permissions, ok := workflowpolicy.MappingValue(job, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode || len(permissions.Content) != 2 {
		return errors.New("native evidence job permissions must contain only contents: read")
	}
	value, ok := workflowpolicy.MappingValue(permissions, "contents")
	if !ok || value.Kind != yaml.ScalarNode || value.Value != "read" {
		return errors.New("native evidence job permissions must contain only contents: read")
	}
	return nil
}

func checkActionReferences(root *yaml.Node) error {
	var references []string
	collectActionReferences(root, &references)
	if len(references) == 0 {
		return errors.New("native evidence workflow must use at least one action")
	}
	for _, reference := range references {
		if !actionReferencePattern.MatchString(reference) {
			return errors.New("every action uses reference must be a full 40-character lowercase SHA")
		}
	}
	return nil
}

func collectActionReferences(node *yaml.Node, references *[]string) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.Value == "uses" && value.Kind == yaml.ScalarNode {
				*references = append(*references, value.Value)
			}
			collectActionReferences(value, references)
		}
		return
	}
	for _, child := range node.Content {
		collectActionReferences(child, references)
	}
}

func checkNoReleaseSideEffects(node *yaml.Node) error {
	var values []string
	collectScalarValues(node, &values)
	for _, value := range values {
		for _, forbidden := range []string{"gh release", "git push", "git tag", "releaseassets finalize", "HOMEBREW_TAP", "RELEASE_SIGNING", "github.token", "GITHUB_TOKEN", "GH_TOKEN", "curl", "wget"} {
			if regexp.MustCompile(`(?i)` + regexp.QuoteMeta(forbidden)).MatchString(value) {
				return fmt.Errorf("native evidence workflow must not contain release side effect %q", forbidden)
			}
		}
	}
	return nil
}

func requireOnlyMappingKeys(mapping *yaml.Node, keys ...string) error {
	if !workflowpolicy.HasExactMappingKeys(mapping, keys...) {
		return errors.New("must contain exactly the audited keys")
	}
	return nil
}

func collectScalarValues(node *yaml.Node, values *[]string) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode {
		*values = append(*values, node.Value)
	}
	for _, child := range node.Content {
		collectScalarValues(child, values)
	}
}
