// 根据已验证 checksums.txt 渲染 URL 与 digest 受约束的 Homebrew formula；子命令分发在 runFormula。

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	versionpkg "github.com/FlanChanXwO/pixiv-cli/internal/version"
)

const (
	projectRepository = "FlanChanXwO/pixiv-cli"
	stableFormula     = "pixiv-cli"
	betaFormula       = "pixiv-cli-beta"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func runFormula(arguments []string) error {
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
	if err := versionpkg.Validate(*version); err != nil {
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

func validateFormulaVersion(formula, version string) (formulaName, templateName string, err error) {
	prerelease := versionpkg.Channel(version) == "prerelease"
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
	targets := FixedTargets()
	expected := make(map[string]struct{}, len(targets)+2)
	for _, target := range targets {
		expected[ArchiveName(version, target)] = struct{}{}
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
	for _, target := range FixedTargets() {
		if target.GOOS == "windows" {
			continue
		}
		archive := ArchiveName(version, target)
		key := strings.ToUpper(target.GOOS + "_" + target.GOARCH)
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

func releaseURL(version, archive string) string {
	return "https://github.com/" + projectRepository + "/releases/download/v" + version + "/" + archive
}

func readRegularFile(path, label string) ([]byte, error) {
	if err := requireRegularFile(path, label); err != nil {
		return nil, err
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
	if err := requireSecureDirectory(parent, "output directory"); err != nil {
		return err
	}
	if err := writeRegularFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write formula output %q: %w", path, err)
	}
	return nil
}
