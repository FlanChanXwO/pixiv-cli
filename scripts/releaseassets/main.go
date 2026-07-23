// Command releaseassets assembles the deterministic artifacts attached to a Pixiv CLI release.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type releaseTarget struct {
	goos   string
	goarch string
}

var fixedTargets = []releaseTarget{
	{goos: "darwin", goarch: "amd64"},
	{goos: "darwin", goarch: "arm64"},
	{goos: "linux", goarch: "amd64"},
	{goos: "linux", goarch: "arm64"},
	{goos: "windows", goarch: "amd64"},
	{goos: "windows", goarch: "arm64"},
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "release assets: %v\n", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("a subcommand is required: validate, channel, package, or finalize")
	}
	switch arguments[0] {
	case "validate":
		return runValidate(arguments[1:])
	case "channel":
		return runChannel(arguments[1:])
	case "package":
		return runPackage(arguments[1:])
	case "finalize":
		return runFinalize(arguments[1:])
	case "-h", "--help", "help":
		return errors.New("usage: releaseassets validate|channel|package|finalize")
	default:
		return fmt.Errorf("unknown subcommand %q", arguments[0])
	}
}

// runChannel 输出 release 的稳定渠道名称。它在 validateVersion 成功后才检查预发布段，
// 因而不会把 build metadata 内合法的连字符误判为 prerelease。
func runChannel(arguments []string) error {
	output, err := channelOutput(append([]string{"channel"}, arguments...))
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(os.Stdout, output)
	return err
}

func channelOutput(arguments []string) (string, error) {
	if len(arguments) == 0 || arguments[0] != "channel" {
		return "", errors.New("channel subcommand is required")
	}
	flags := flag.NewFlagSet("channel", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.String("version", "", "semantic version without v")
	if err := flags.Parse(arguments[1:]); err != nil {
		return "", err
	}
	if flags.NArg() != 0 {
		return "", fmt.Errorf("channel accepts no positional arguments: %q", flags.Arg(0))
	}
	if err := validateVersion(*version); err != nil {
		return "", err
	}

	// validateVersion 已验证整个 SemVer 语法；移除 build metadata 后出现的连字符只能是
	// 预发布分隔符，无需复制另一套 SemVer parser。
	coreVersion, _, _ := strings.Cut(*version, "+")
	if strings.Contains(coreVersion, "-") {
		return "prerelease\n", nil
	}
	return "stable\n", nil
}

// runValidate 让 workflow 能在拉取 source 后、开始六个平台构建前复用唯一的
// SemVer 规则拒绝无效 tag，避免错误标签耗费 runner 或触及发布凭据。
func runValidate(arguments []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.String("version", "", "semantic version without v")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("validate accepts no positional arguments: %q", flags.Arg(0))
	}
	return validateVersion(*version)
}

func runPackage(arguments []string) error {
	flags := flag.NewFlagSet("package", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	repoRoot := flags.String("repo-root", "", "repository root")
	version := flags.String("version", "", "semantic version without v")
	targetText := flags.String("target", "", "platform target in GOOS/GOARCH form")
	binary := flags.String("binary", "", "built pixiv executable")
	outputDir := flags.String("output-dir", "", "existing directory receiving the archive")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("package accepts no positional arguments: %q", flags.Arg(0))
	}
	if err := validateVersion(*version); err != nil {
		return err
	}
	target, err := parseTarget(*targetText)
	if err != nil {
		return err
	}
	if err := requireRegularFile(*binary, "binary"); err != nil {
		return err
	}
	if err := requireDirectory(*outputDir, "output directory"); err != nil {
		return err
	}
	root, err := requireRepositoryRoot(*repoRoot)
	if err != nil {
		return err
	}

	archivePath := filepath.Join(*outputDir, archiveName(*version, target))
	if _, err := os.Lstat(archivePath); err == nil {
		return fmt.Errorf("release archive already exists: %s", archivePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect release archive output %q: %w", archivePath, err)
	}
	packager := filepath.Join(root, "scripts", "package-release.sh")
	if err := requireRegularFile(packager, "release packager"); err != nil {
		return err
	}
	format := "tar.gz"
	if target.goos == "windows" {
		format = "zip"
	}
	command := exec.Command("sh", packager, "--binary", *binary, "--format", format, "--output", archivePath)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("package %s: %w", archiveName(*version, target), err)
	}
	if err := requireRegularFile(archivePath, "packaged release archive"); err != nil {
		return err
	}
	return nil
}

func runFinalize(arguments []string) (err error) {
	flags := flag.NewFlagSet("finalize", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.String("version", "", "semantic version without v")
	inputDir := flags.String("input-dir", "", "directory containing the six packaged archives")
	outputDir := flags.String("output-dir", "", "new directory receiving final release assets")
	changelog := flags.String("changelog", "", "version-specific English changelog path")
	changelogChinese := flags.String("changelog-zh", "", "version-specific Simplified Chinese changelog path")
	installSh := flags.String("install-sh", "", "verified install.sh path")
	installCmd := flags.String("install-cmd", "", "verified install.cmd path")
	privateKeyPath := flags.String("private-key", "", "PKCS#8 PEM Ed25519 private-key path")
	keyID := flags.String("key-id", "", "public signing-key identifier")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("finalize accepts no positional arguments: %q", flags.Arg(0))
	}
	if err := validateVersion(*version); err != nil {
		return err
	}
	if strings.TrimSpace(*keyID) == "" {
		return errors.New("key ID is required")
	}
	if err := requireSecureDirectory(*inputDir, "input directory"); err != nil {
		return err
	}
	if err := requireSecureRegularFile(*changelog, "changelog"); err != nil {
		return err
	}
	if err := requireSecureRegularFile(*changelogChinese, "Simplified Chinese changelog"); err != nil {
		return err
	}
	if err := requireSecureRegularFile(*privateKeyPath, "private key"); err != nil {
		return err
	}
	privateKey, err := readEd25519PrivateKey(*privateKeyPath)
	if err != nil {
		return err
	}
	englishNotes, err := releaseNotesFromChangelog(*changelog, *version)
	if err != nil {
		return err
	}
	chineseNotes, err := releaseNotesFromChangelog(*changelogChinese, *version)
	if err != nil {
		return err
	}
	notes := bilingualReleaseNotes(englishNotes, chineseNotes)
	if err := requireSecureRegularFile(*installSh, "install.sh"); err != nil {
		return err
	}
	if err := requireSecureRegularFile(*installCmd, "install.cmd"); err != nil {
		return err
	}
	parent := filepath.Dir(*outputDir)
	if err := requireSecureDirectory(parent, "output parent directory"); err != nil {
		return err
	}
	if _, statErr := os.Lstat(*outputDir); statErr == nil {
		return fmt.Errorf("release output directory already exists: %s", *outputDir)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect release output directory %q: %w", *outputDir, statErr)
	}

	stage, err := os.MkdirTemp(parent, ".pixiv-release-assets-")
	if err != nil {
		return fmt.Errorf("create release output staging directory: %w", err)
	}
	defer func() {
		if stage == "" {
			return
		}
		if removeErr := os.RemoveAll(stage); removeErr != nil && err == nil {
			err = fmt.Errorf("remove failed release output staging directory %q: %w", stage, removeErr)
		}
	}()

	checksums, err := copyRequiredAssets(*inputDir, stage, *version, *installSh, *installCmd)
	if err != nil {
		return err
	}
	if err := writeRegularFile(filepath.Join(stage, "checksums.txt"), checksums, 0o644); err != nil {
		return fmt.Errorf("write checksums.txt: %w", err)
	}
	manifest, err := signedChecksumsManifest(*keyID, privateKey, checksums)
	if err != nil {
		return err
	}
	if err := writeRegularFile(filepath.Join(stage, "checksums.json"), manifest, 0o644); err != nil {
		return fmt.Errorf("write checksums.json: %w", err)
	}
	if err := writeRegularFile(filepath.Join(stage, "release-notes.md"), notes, 0o644); err != nil {
		return fmt.Errorf("write release-notes.md: %w", err)
	}
	if err := os.Rename(stage, *outputDir); err != nil {
		return fmt.Errorf("publish release output directory %q: %w", *outputDir, err)
	}
	stage = ""
	return nil
}

func copyRequiredAssets(inputDir, outputDir, version, installSh, installCmd string) ([]byte, error) {
	sources := make(map[string]string, len(fixedTargets)+2)
	for _, target := range fixedTargets {
		name := archiveName(version, target)
		sources[name] = filepath.Join(inputDir, name)
	}
	sources["install.sh"] = installSh
	sources["install.cmd"] = installCmd
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	checksums := make([]byte, 0, len(names)*96)
	for _, name := range names {
		source := sources[name]
		if err := requireRegularFile(source, "input release asset"); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		sum, err := copyRegularFile(source, filepath.Join(outputDir, name))
		if err != nil {
			return nil, fmt.Errorf("copy %s: %w", name, err)
		}
		checksums = append(checksums, hex.EncodeToString(sum[:])...)
		checksums = append(checksums, "  "...)
		checksums = append(checksums, name...)
		checksums = append(checksums, '\n')
	}
	return checksums, nil
}

func copyRegularFile(source, destination string) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	before, err := os.Lstat(source)
	if err != nil {
		return zero, fmt.Errorf("inspect source: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return zero, errors.New("source must be a non-symlink regular file")
	}
	input, err := os.Open(source)
	if err != nil {
		return zero, fmt.Errorf("open source: %w", err)
	}
	defer input.Close()
	after, err := input.Stat()
	if err != nil {
		return zero, fmt.Errorf("stat opened source: %w", err)
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return zero, errors.New("source changed while opening")
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return zero, fmt.Errorf("create destination: %w", err)
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
		return zero, fmt.Errorf("copy contents: %w", err)
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return zero, fmt.Errorf("sync destination: %w", err)
	}
	if err := output.Close(); err != nil {
		return zero, fmt.Errorf("close destination: %w", err)
	}
	failed = false
	var sum [sha256.Size]byte
	copy(sum[:], hasher.Sum(nil))
	return sum, nil
}

func readEd25519PrivateKey(path string) (ed25519.PrivateKey, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, rest := pem.Decode(body)
	if block == nil || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("private key must contain exactly one PKCS#8 PEM block")
	}
	if block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("private key PEM type must be PRIVATE KEY, got %q", block.Type)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("PKCS#8 private key must be Ed25519")
	}
	return privateKey, nil
}

func releaseNotesFromChangelog(path, version string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read changelog: %w", err)
	}
	header := regexp.MustCompile(`(?m)^# v` + regexp.QuoteMeta(version) + `(?:\s+[—-]\s+.+)?\s*$`)
	location := header.FindIndex(body)
	if location == nil {
		return nil, fmt.Errorf("changelog has no release heading for v%s", version)
	}
	if second := header.FindIndex(body[location[1]:]); second != nil {
		return nil, fmt.Errorf("changelog has more than one release heading for v%s", version)
	}
	remainder := body[location[1]:]
	notes := strings.Trim(string(remainder), "\n")
	if notes == "" {
		return nil, fmt.Errorf("changelog release v%s has no release notes", version)
	}
	return []byte(notes + "\n"), nil
}

// bilingualReleaseNotes 为 GitHub Release body 增加稳定的语言标题。两个输入都已经
// 由 releaseNotesFromChangelog 验证了对应版本标题，并且不会进入签名 checksum asset 集合。
func bilingualReleaseNotes(english, chinese []byte) []byte {
	return []byte("# English\n\n" + strings.TrimSpace(string(english)) + "\n\n---\n\n# 简体中文\n\n" + strings.TrimSpace(string(chinese)) + "\n")
}

func signedChecksumsManifest(keyID string, privateKey ed25519.PrivateKey, checksums []byte) ([]byte, error) {
	sum := sha256.Sum256(checksums)
	manifest := struct {
		KeyID           string `json:"key_id"`
		ChecksumsSHA256 string `json:"checksums_sha256"`
		Signature       string `json:"signature"`
	}{
		KeyID:           keyID,
		ChecksumsSHA256: hex.EncodeToString(sum[:]),
		Signature:       base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, checksums)),
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode checksums manifest: %w", err)
	}
	return append(body, '\n'), nil
}

func writeRegularFile(path string, contents []byte, permissions os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, permissions)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
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

func validateVersion(version string) error {
	if !semanticVersionPattern.MatchString(version) {
		return fmt.Errorf("version must be a semantic version without a leading v: %q", version)
	}
	return nil
}

func parseTarget(value string) (releaseTarget, error) {
	for _, target := range fixedTargets {
		if value == target.goos+"/"+target.goarch {
			return target, nil
		}
	}
	available := make([]string, 0, len(fixedTargets))
	for _, target := range fixedTargets {
		available = append(available, target.goos+"/"+target.goarch)
	}
	sort.Strings(available)
	return releaseTarget{}, fmt.Errorf("target must be one of %s, got %q", strings.Join(available, ", "), value)
}

func archiveName(version string, target releaseTarget) string {
	extension := ".tar.gz"
	if target.goos == "windows" {
		extension = ".zip"
	}
	return "pixiv-cli_" + version + "_" + target.goos + "_" + target.goarch + extension
}

func requireRepositoryRoot(path string) (string, error) {
	if err := requireDirectory(path, "repository root"); err != nil {
		return "", err
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repository root %q: %w", path, err)
	}
	if err := requireRegularFile(filepath.Join(root, "go.mod"), "repository go.mod"); err != nil {
		return "", err
	}
	return root, nil
}

func requireRegularFile(path, label string) error {
	if path == "" {
		return fmt.Errorf("%s is required", label)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s %q: %w", label, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a non-symlink regular file: %q", label, path)
	}
	return nil
}

func requireDirectory(path, label string) error {
	if path == "" {
		return fmt.Errorf("%s is required", label)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s %q: %w", label, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s must be a non-symlink directory: %q", label, path)
	}
	return nil
}

func requireSecureRegularFile(path, label string) error {
	if err := requireRegularFile(path, label); err != nil {
		return err
	}
	if err := rejectSymlinkAncestors(filepath.Dir(path)); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func requireSecureDirectory(path, label string) error {
	if err := requireDirectory(path, label); err != nil {
		return err
	}
	if err := rejectSymlinkAncestors(path); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

// rejectSymlinkAncestors keeps an artifact write or secret read inside the caller's
// declared directory tree. Existing paths are walked lexically, before any
// operation that would otherwise follow a symlink.
func rejectSymlinkAncestors(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", path, err)
	}
	ancestors := make([]string, 0)
	for current := filepath.Clean(absolute); ; current = filepath.Dir(current) {
		ancestors = append(ancestors, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	for index := len(ancestors) - 1; index >= 0; index-- {
		info, err := os.Lstat(ancestors[index])
		if err != nil {
			return fmt.Errorf("inspect path ancestor %q: %w", ancestors[index], err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path contains a symlink ancestor: %s", ancestors[index])
		}
	}
	return nil
}
