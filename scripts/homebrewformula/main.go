// Command homebrewformula 根据已验证 release checksums.txt 的六个 archive
// 渲染 URL 与 digest 均受约束的 Homebrew formula。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	projectRepository = "FlanChanXwO/pixiv-cli"
	stableFormula     = "pixiv-cli"
	betaFormula       = "pixiv-cli-beta"
)

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

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
		fmt.Fprintf(os.Stderr, "homebrew formula: %v\n", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("a subcommand is required: render")
	}
	if arguments[0] != "render" {
		return fmt.Errorf("unknown subcommand %q; expected render", arguments[0])
	}
	return runRender(arguments[1:])
}

// runRender 是供发布 workflow 调用的唯一公开入口：release 产物的 checksums.txt
// 是 version、URL 与 SHA-256 唯一来源，模板本身不保存任何版本或 digest。
func runRender(arguments []string) error {
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	formula := flags.String("formula", "", "target formula: pixiv-cli or pixiv-cli-beta")
	version := flags.String("version", "", "release semantic version without v")
	checksumsPath := flags.String("checksums", "", "release checksums.txt path")
	outputPath := flags.String("output", "", "new Ruby formula output path")
	templatesDir := flags.String("templates", filepath.Join("templates", "homebrew"), "formula template directory")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("render accepts no positional arguments: %q", flags.Arg(0))
	}
	if err := validateVersion(*version); err != nil {
		return err
	}
	formulaName, templateName, err := validateFormulaVersion(*formula, *version)
	if err != nil {
		return err
	}
	checksums, err := readChecksums(*checksumsPath, *version)
	if err != nil {
		return err
	}
	templatePath := filepath.Join(*templatesDir, templateName)
	template, err := readRegularFile(templatePath, "formula template")
	if err != nil {
		return err
	}
	body, err := renderFormula(string(template), formulaName, *version, checksums)
	if err != nil {
		return err
	}
	return writeNewRegularFile(*outputPath, []byte(body))
}

func validateVersion(version string) error {
	if !semanticVersionPattern.MatchString(version) {
		return fmt.Errorf("version must be a semantic version without a leading v: %q", version)
	}
	return nil
}

func validateFormulaVersion(formula, version string) (formulaName, templateName string, err error) {
	base := strings.SplitN(version, "+", 2)[0]
	prerelease := strings.Contains(base, "-")
	switch formula {
	case stableFormula:
		if prerelease {
			return "", "", fmt.Errorf("stable formula %q only accepts a stable semantic version, got %q", stableFormula, version)
		}
		return stableFormula, "pixiv-cli.rb.tmpl", nil
	case betaFormula:
		if !prerelease {
			return "", "", fmt.Errorf("beta formula %q only accepts a prerelease semantic version, got %q", betaFormula, version)
		}
		return betaFormula, "pixiv-cli-beta.rb.tmpl", nil
	default:
		return "", "", fmt.Errorf("formula must be %q or %q, got %q", stableFormula, betaFormula, formula)
	}
}

func readChecksums(path, version string) (map[string]string, error) {
	body, err := readRegularFile(path, "checksums file")
	if err != nil {
		return nil, err
	}
	expected := make(map[string]struct{}, len(fixedTargets)+2)
	for _, target := range fixedTargets {
		expected[archiveName(version, target)] = struct{}{}
	}
	expected["install.cmd"] = struct{}{}
	expected["install.sh"] = struct{}{}
	checksums := make(map[string]string, len(expected))
	lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, errors.New("checksums file is empty")
	}
	for lineNumber, line := range lines {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || !sha256Pattern.MatchString(parts[0]) {
			return nil, fmt.Errorf("checksums file line %d must be '<64 lowercase hex>  <asset>'", lineNumber+1)
		}
		if _, ok := expected[parts[1]]; !ok {
			return nil, fmt.Errorf("checksums file line %d names unexpected asset %q", lineNumber+1, parts[1])
		}
		if _, duplicate := checksums[parts[1]]; duplicate {
			return nil, fmt.Errorf("checksums file has duplicate entry for %q", parts[1])
		}
		checksums[parts[1]] = parts[0]
	}
	for name := range expected {
		if _, ok := checksums[name]; !ok {
			return nil, fmt.Errorf("checksums file has no entry for required release asset %q", name)
		}
	}
	return checksums, nil
}

func renderFormula(template, formula, version string, checksums map[string]string) (string, error) {
	formulaClass := "PixivCli"
	if formula == betaFormula {
		formulaClass = "PixivCliBeta"
	}
	values := map[string]string{
		"{{FORMULA_CLASS}}": formulaClass,
		"{{VERSION}}":       version,
	}
	for _, target := range fixedTargets {
		if target.goos == "windows" {
			continue
		}
		archive := archiveName(version, target)
		key := strings.ToUpper(target.goos + "_" + target.goarch)
		values["{{"+key+"_URL}}"] = releaseURL(version, archive)
		values["{{"+key+"_SHA256}}"] = checksums[archive]
	}
	result := template
	for placeholder, value := range values {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	if strings.Contains(result, "{{") || strings.Contains(result, "}}") {
		return "", errors.New("formula template has an unrecognized placeholder")
	}
	return result, nil
}

func archiveName(version string, target releaseTarget) string {
	extension := ".tar.gz"
	if target.goos == "windows" {
		extension = ".zip"
	}
	return "pixiv-cli_" + version + "_" + target.goos + "_" + target.goarch + extension
}

func releaseURL(version, archive string) string {
	return "https://github.com/" + projectRepository + "/releases/download/v" + version + "/" + archive
}

func readRegularFile(path, label string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("%s is required", label)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %s %q: %w", label, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a non-symlink regular file: %q", label, path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s %q: %w", label, path, err)
	}
	return body, nil
}

func writeNewRegularFile(path string, body []byte) error {
	if path == "" {
		return errors.New("output is required")
	}
	parent := filepath.Dir(path)
	info, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("inspect output directory %q: %w", parent, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("output directory must be a non-symlink directory: %q", parent)
	}
	if err := rejectSymlinkAncestors(parent); err != nil {
		return fmt.Errorf("output directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create formula output %q: %w", path, err)
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write formula output %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("sync formula output %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close formula output %q: %w", path, err)
	}
	return nil
}

// rejectSymlinkAncestors 在创建 formula 前按词法路径检查每个既有祖先。filepath.Abs
// 不会跟随 symlink；因此可在 OpenFile 之前拒绝 link/real-child 这类间接穿透。
func rejectSymlinkAncestors(path string) error {
	ancestor, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve output directory %q: %w", path, err)
	}
	for {
		info, err := os.Lstat(ancestor)
		if err != nil {
			return fmt.Errorf("inspect output directory ancestor %q: %w", ancestor, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output directory contains a symlink ancestor: %q", ancestor)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return nil
		}
		ancestor = parent
	}
}
