package main

import (
	"bytes"
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
	"sort"
	"strings"

	versionpkg "github.com/FlanChanXwO/pixiv-cli/internal/version"
	"gopkg.in/yaml.v3"
)

var fixedTargets = FixedTargets()

func runAssets(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("a subcommand is required: validate, validate-source, channel, package, or finalize")
	}
	switch arguments[0] {
	case "validate":
		return runAssetsValidate(arguments[1:])
	case "validate-source":
		return runValidateSource(arguments[1:])
	case "channel":
		return runChannel(arguments[1:])
	case "package":
		return runPackage(arguments[1:])
	case "finalize":
		return runFinalize(arguments[1:])
	case "-h", "--help", "help":
		return errors.New("usage: releaseassets validate|validate-source|channel|package|finalize")
	default:
		return fmt.Errorf("unknown subcommand %q", arguments[0])
	}
}

func runChannel(arguments []string) error {
	flags := flag.NewFlagSet("channel", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.String("version", "", "semantic version without v")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("channel accepts no positional arguments: %q", flags.Arg(0))
	}
	if err := versionpkg.Validate(*version); err != nil {
		return err
	}
	_, err := fmt.Fprintln(os.Stdout, versionpkg.Channel(*version))
	return err
}

// runAssetsValidate 在六个平台构建前用同一套 SemVer 规则拒绝无效 tag，避免错误标签
// 耗费 runner 或触及发布凭据。
func runAssetsValidate(arguments []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.String("version", "", "semantic version without v")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("validate accepts no positional arguments: %q", flags.Arg(0))
	}
	return versionpkg.Validate(*version)
}

// runValidateSource 在 SemVer 之外校验待发布 source 的产品 Skill metadata。SkillHub 和
// ClawHub 都以该 metadata 的版本发布，所以它必须与不可变 release tag 一致。
func runValidateSource(arguments []string) error {
	flags := flag.NewFlagSet("validate-source", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.String("version", "", "semantic version without v")
	productSkill := flags.String("product-skill", "", "path to the released product SKILL.md")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *productSkill == "" {
		return errors.New("--product-skill is required")
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("validate-source accepts no positional arguments: %q", flags.Arg(0))
	}
	if err := versionpkg.Validate(*version); err != nil {
		return err
	}
	skillVersion, err := productSkillVersion(*productSkill)
	if err != nil {
		return err
	}
	if skillVersion != *version {
		return fmt.Errorf("product skill version %q does not match release version %q", skillVersion, *version)
	}
	return nil
}

// productSkillVersion 只读取 SKILL.md 的 YAML front matter 中唯一的 version 字段。
// YAML decoder 兼容有效的字段顺序、注释和引号；正文不参与解析，示例或说明不会改变
// 发布身份。
func productSkillVersion(path string) (string, error) {
	if err := requireRegularFile(path, "product skill"); err != nil {
		return "", err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read product skill %q: %w", path, err)
	}
	lines := strings.Split(string(contents), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return "", fmt.Errorf("product skill %q must start with YAML front matter", path)
	}
	frontMatterEnd := -1
	for index, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			frontMatterEnd = index + 1
			break
		}
	}
	if frontMatterEnd == -1 {
		return "", fmt.Errorf("product skill %q has unterminated YAML front matter", path)
	}
	var metadata struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:frontMatterEnd], "\n")), &metadata); err != nil {
		return "", fmt.Errorf("parse product skill %q YAML front matter: %w", path, err)
	}
	if metadata.Version == "" {
		return "", fmt.Errorf("product skill %q has no version", path)
	}
	if err := versionpkg.Validate(metadata.Version); err != nil {
		return "", fmt.Errorf("product skill %q has invalid version: %w", path, err)
	}
	return metadata.Version, nil
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
	if err := versionpkg.Validate(*version); err != nil {
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

	archivePath := filepath.Join(*outputDir, ArchiveName(*version, target))
	if _, err := os.Lstat(archivePath); err == nil {
		return fmt.Errorf("release archive already exists: %s", archivePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect release archive output %q: %w", archivePath, err)
	}
	packager := filepath.Join(root, "scripts", "build", "package.sh")
	if err := requireRegularFile(packager, "release packager"); err != nil {
		return err
	}
	format := "tar.gz"
	if target.GOOS == "windows" {
		format = "zip"
	}
	command := exec.Command("sh", packager, "--binary", *binary, "--format", format, "--output", archivePath)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("package %s: %w", ArchiveName(*version, target), err)
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
	releaseSourcesPath := flags.String("release-sources", "", "versioned release-source list injected into install.sh")
	privateKeyPath := flags.String("private-key", "", "PKCS#8 PEM Ed25519 private-key path")
	keyID := flags.String("key-id", "", "public signing-key identifier")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("finalize accepts no positional arguments: %q", flags.Arg(0))
	}
	// 1. 在任何写入之前校验版本、密钥和全部输入路径；rejectSymlinkAncestors 保证产物
	//    只落在调用方声明的目录树内。
	if err := versionpkg.Validate(*version); err != nil {
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
	englishNotes, err := NotesFromChangelog(*changelog, *version)
	if err != nil {
		return err
	}
	chineseNotes, err := NotesFromChangelog(*changelogChinese, *version)
	if err != nil {
		return err
	}
	notes := BilingualBody(englishNotes, chineseNotes)
	if err := requireSecureRegularFile(*installSh, "install.sh"); err != nil {
		return err
	}
	if err := requireSecureRegularFile(*installCmd, "install.cmd"); err != nil {
		return err
	}
	var releaseSources []byte
	if *releaseSourcesPath != "" {
		if err := requireSecureRegularFile(*releaseSourcesPath, "release sources"); err != nil {
			return err
		}
		releaseSources, err = os.ReadFile(*releaseSourcesPath)
		if err != nil {
			return fmt.Errorf("read release sources %q: %w", *releaseSourcesPath, err)
		}
		if len(bytes.TrimSpace(releaseSources)) == 0 {
			return errors.New("release sources must not be empty")
		}
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

	// 2. 产物先写进 MkdirTemp 生成的父目录临时目录；中途失败时由 defer 删除，避免
	//    留下半个发布目录。
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

	// 3. 按固定顺序复制归档并注入安装器候选，checksums 覆盖的是注入之后的字节。
	checksums, err := copyRequiredAssets(*inputDir, stage, *version, *installSh, *installCmd, releaseSources)
	if err != nil {
		return err
	}
	if err := writeRegularFile(filepath.Join(stage, "checksums.txt"), checksums, 0o644); err != nil {
		return fmt.Errorf("write checksums.txt: %w", err)
	}
	// 4. 对 step 3 的 checksums 签名，并把双语发布说明一并写入暂存目录。
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
	// 5. rename 让输出目录要么不存在、要么完整；成功后清空 stage，避免 defer 删除它。
	if err := os.Rename(stage, *outputDir); err != nil {
		return fmt.Errorf("publish release output directory %q: %w", *outputDir, err)
	}
	stage = ""
	return nil
}

func copyRequiredAssets(inputDir, outputDir, version, installSh, installCmd string, releaseSources []byte) ([]byte, error) {
	sources := make(map[string]string, len(fixedTargets)+2)
	for _, target := range fixedTargets {
		name := ArchiveName(version, target)
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
		destination := filepath.Join(outputDir, name)
		sum, err := copyRegularFile(source, destination)
		if err != nil {
			return nil, fmt.Errorf("copy %s: %w", name, err)
		}
		if name == "install.sh" && len(releaseSources) != 0 {
			sum, err = injectReleaseSources(destination, releaseSources)
			if err != nil {
				return nil, fmt.Errorf("inject release sources into install.sh: %w", err)
			}
		}
		if name == "install.cmd" && len(releaseSources) != 0 {
			sum, err = injectWindowsReleaseSources(destination, releaseSources)
			if err != nil {
				return nil, fmt.Errorf("inject release sources into install.cmd: %w", err)
			}
		}
		checksums = append(checksums, hex.EncodeToString(sum[:])...)
		checksums = append(checksums, "  "...)
		checksums = append(checksums, name...)
		checksums = append(checksums, '\n')
	}
	return checksums, nil
}

const (
	releaseSourcesBeginMarker      = "# PIXIV_RELEASE_SOURCES_BEGIN\n"
	releaseSourcesEndMarker        = "# PIXIV_RELEASE_SOURCES_END"
	releaseSourcesHeredoc          = "PIXIV_RELEASE_SOURCES_EOF"
	windowsReleaseSourcesBeginLine = "rem PIXIV_RELEASE_SOURCES_BEGIN"
	windowsReleaseSourcesEndMarker = "rem PIXIV_RELEASE_SOURCES_END"
)

func injectReleaseSources(path string, sources []byte) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	payload, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("read staged installer: %w", err)
	}
	content := string(payload)
	start := strings.Index(content, releaseSourcesBeginMarker)
	if start < 0 {
		return zero, errors.New("release-source begin marker is missing")
	}
	endOffset := strings.Index(content[start+len(releaseSourcesBeginMarker):], releaseSourcesEndMarker)
	if endOffset < 0 {
		return zero, errors.New("release-source end marker is missing")
	}
	end := start + len(releaseSourcesBeginMarker) + endOffset
	trimmed := strings.TrimSuffix(string(sources), "\n")
	if strings.Contains(trimmed, "\n"+releaseSourcesHeredoc) || strings.HasPrefix(trimmed, releaseSourcesHeredoc+"\n") || trimmed == releaseSourcesHeredoc {
		return zero, fmt.Errorf("release sources contain reserved heredoc delimiter %q", releaseSourcesHeredoc)
	}
	replacement := releaseSourcesBeginMarker + "release_sources=$(cat <<'" + releaseSourcesHeredoc + "'\n" + trimmed + "\n" + releaseSourcesHeredoc + "\n)\n"
	rendered := content[:start] + replacement + content[end:]
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		return zero, fmt.Errorf("write rendered installer: %w", err)
	}
	return sha256.Sum256([]byte(rendered)), nil
}

func injectWindowsReleaseSources(path string, sources []byte) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	payload, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("read staged installer: %w", err)
	}
	content := string(payload)
	lineEnding := "\r\n"
	beginMarker := windowsReleaseSourcesBeginLine + lineEnding
	if !strings.Contains(content, beginMarker) {
		lineEnding = "\n"
		beginMarker = windowsReleaseSourcesBeginLine + lineEnding
	}
	start := strings.Index(content, beginMarker)
	if start < 0 {
		return zero, errors.New("release-source begin marker is missing")
	}
	endOffset := strings.Index(content[start+len(beginMarker):], windowsReleaseSourcesEndMarker)
	if endOffset < 0 {
		return zero, errors.New("release-source end marker is missing")
	}
	entries, err := windowsReleaseSourceEntries(sources)
	if err != nil {
		return zero, err
	}
	end := start + len(beginMarker) + endOffset
	var replacement strings.Builder
	replacement.WriteString(beginMarker)
	fmt.Fprintf(&replacement, "set \"RELEASE_SOURCE_COUNT=%d\"%s", len(entries), lineEnding)
	for index, entry := range entries {
		fmt.Fprintf(&replacement, "set \"RELEASE_SOURCE_%d=%s\"%s", index+1, entry, lineEnding)
	}
	rendered := content[:start] + replacement.String() + content[end:]
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		return zero, fmt.Errorf("write rendered installer: %w", err)
	}
	return sha256.Sum256([]byte(rendered)), nil
}

func windowsReleaseSourceEntries(sources []byte) ([]string, error) {
	var entries []string
	for lineNumber, line := range strings.Split(strings.ReplaceAll(string(sources), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(strings.Split(line, "|")) != 3 {
			return nil, fmt.Errorf("release source line %d must have three fields", lineNumber+1)
		}
		if strings.ContainsAny(line, "\r\n\"%") {
			return nil, fmt.Errorf("release source line %d contains unsupported Windows batch characters", lineNumber+1)
		}
		entries = append(entries, line)
	}
	if len(entries) == 0 {
		return nil, errors.New("release sources must contain at least one entry")
	}
	return entries, nil
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

func parseTarget(value string) (Target, error) {
	for _, target := range fixedTargets {
		if value == target.String() {
			return target, nil
		}
	}
	available := make([]string, 0, len(fixedTargets))
	for _, target := range fixedTargets {
		available = append(available, target.String())
	}
	sort.Strings(available)
	return Target{}, fmt.Errorf("target must be one of %s, got %q", strings.Join(available, ", "), value)
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

// rejectSymlinkAncestors 把产物写入和密钥读取限制在调用方声明的目录树内。已存在的路径
// 在任何会跟随 symlink 的操作之前按词法逐级检查。
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
