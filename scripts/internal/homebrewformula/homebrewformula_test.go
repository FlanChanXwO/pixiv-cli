package homebrewformula

import (
	"strings"
	"testing"
)

func TestRenderFormulaFillsStablePlaceholders(t *testing.T) {
	t.Parallel()

	template := strings.Join([]string{
		"class {{FORMULA_CLASS}} < Formula",
		"  version \"{{VERSION}}\"",
		"  url \"{{DARWIN_AMD64_URL}}\"",
		"  sha256 \"{{DARWIN_AMD64_SHA256}}\"",
		"",
	}, "\n")
	checksums := map[string]string{
		"pixiv-cli_0.1.0_darwin_amd64.tar.gz": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"pixiv-cli_0.1.0_darwin_arm64.tar.gz": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"pixiv-cli_0.1.0_linux_amd64.tar.gz":  "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		"pixiv-cli_0.1.0_linux_arm64.tar.gz":  "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		"pixiv-cli_0.1.0_windows_amd64.zip":   "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"pixiv-cli_0.1.0_windows_arm64.zip":   "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"install.sh":                          "1111111111111111111111111111111111111111111111111111111111111111",
		"install.cmd":                         "2222222222222222222222222222222222222222222222222222222222222222",
	}
	rendered, err := renderFormula(template, "pixiv-cli", "0.1.0", checksums)
	if err != nil {
		t.Fatalf("renderFormula() error = %v", err)
	}
	for _, want := range []string{
		"class PixivCli < Formula",
		`version "0.1.0"`,
		`url "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v0.1.0/pixiv-cli_0.1.0_darwin_amd64.tar.gz"`,
		`sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("renderFormula() = %q, missing %q", rendered, want)
		}
	}
	if strings.Contains(rendered, "{{") {
		t.Fatalf("renderFormula() left an unrecognized placeholder: %q", rendered)
	}
}

func TestRenderFormulaRejectsUnrecognizedPlaceholder(t *testing.T) {
	t.Parallel()

	_, err := renderFormula("class PixivCli\n  url \"{{DARWIN_AMD64_URL}}\"\n  secret \"{{UNKNOWN}}\"\n", "pixiv-cli", "0.1.0", map[string]string{
		"pixiv-cli_0.1.0_darwin_amd64.tar.gz": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"pixiv-cli_0.1.0_darwin_arm64.tar.gz": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"pixiv-cli_0.1.0_linux_amd64.tar.gz":  "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		"pixiv-cli_0.1.0_linux_arm64.tar.gz":  "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		"pixiv-cli_0.1.0_windows_amd64.zip":   "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"pixiv-cli_0.1.0_windows_arm64.zip":   "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"install.sh":                          "1111111111111111111111111111111111111111111111111111111111111111",
		"install.cmd":                         "2222222222222222222222222222222222222222222222222222222222222222",
	})
	if err == nil || !strings.Contains(err.Error(), "unrecognized placeholder") {
		t.Fatalf("renderFormula() error = %v, want unrecognized-placeholder error", err)
	}
}

func TestValidateFormulaVersionRoutesStableAndBeta(t *testing.T) {
	t.Parallel()

	formulaName, templateName, err := validateFormulaVersion("pixiv-cli", "0.1.0")
	if err != nil || formulaName != "pixiv-cli" || templateName != "pixiv-cli.rb.tmpl" {
		t.Fatalf("validateFormulaVersion(stable) = (%q, %q, %v)", formulaName, templateName, err)
	}
	formulaName, templateName, err = validateFormulaVersion("pixiv-cli-beta", "0.2.0-rc.1")
	if err != nil || formulaName != "pixiv-cli-beta" || templateName != "pixiv-cli-beta.rb.tmpl" {
		t.Fatalf("validateFormulaVersion(beta) = (%q, %q, %v)", formulaName, templateName, err)
	}
	if _, _, err := validateFormulaVersion("pixiv-cli", "0.2.0-rc.1"); err == nil {
		t.Fatal("validateFormulaVersion accepted prerelease for stable formula")
	}
	if _, _, err := validateFormulaVersion("pixiv-cli-beta", "0.2.0"); err == nil {
		t.Fatal("validateFormulaVersion accepted stable version for beta formula")
	}
}
