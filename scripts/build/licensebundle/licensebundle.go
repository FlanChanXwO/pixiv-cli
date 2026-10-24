// 许可证 bundle 的生成与校验：六目标生产模块、归属声明、索引与完整文本树。
// 入口在 main.go，依赖枚举在 dependencies.go。

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// releaseTarget 是一个发布目标；licensebundle 不依赖 scripts/release 的发布身份包。
type releaseTarget struct {
	GOOS   string
	GOARCH string
}

func (t releaseTarget) String() string {
	return t.GOOS + "/" + t.GOARCH
}

var releaseTargets = []releaseTarget{
	{GOOS: "darwin", GOARCH: "amd64"},
	{GOOS: "darwin", GOARCH: "arm64"},
	{GOOS: "linux", GOARCH: "amd64"},
	{GOOS: "linux", GOARCH: "arm64"},
	{GOOS: "windows", GOARCH: "amd64"},
	{GOOS: "windows", GOARCH: "arm64"},
}

type packageMetadata struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	License    string `json:"license,omitempty"`
	Repository string `json:"repository,omitempty"`
	Directory  string `json:"directory"`
	Note       string `json:"note,omitempty"`
}

type licenseFile struct {
	Name string
	Body []byte
}

type bundledPackage struct {
	Package packageMetadata
	Files   []licenseFile
}

type options struct {
	Index      string
	Licenses   string
	Check      bool
	Go         string
	Repository string
}

func run(args []string) error {
	flags := flag.NewFlagSet("licensebundle", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	opts := options{}
	flags.StringVar(&opts.Index, "index", "THIRD_PARTY_LICENSES.md", "generated bundle index")
	flags.StringVar(&opts.Licenses, "licenses-dir", "third_party/licenses", "generated license text directory")
	flags.BoolVar(&opts.Check, "check", false, "verify generated files without modifying them")
	flags.StringVar(&opts.Go, "go", "go", "Go executable")
	flags.StringVar(&opts.Repository, "repository", ".", "repository root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	return generate(opts)
}

func generate(opts options) error {
	repository, err := filepath.Abs(opts.Repository)
	if err != nil {
		return err
	}
	index, err := repositoryOutputPath(repository, opts.Index, "index")
	if err != nil {
		return err
	}
	licenses, err := repositoryOutputPath(repository, opts.Licenses, "licenses directory")
	if err != nil {
		return err
	}
	if bundleOutputPathsOverlap(index, licenses) {
		return fmt.Errorf("index and licenses directory output paths must not overlap")
	}
	bundle, err := collectDependencies(opts.Go, repository)
	if err != nil {
		return err
	}
	if opts.Check {
		temporary, err := os.MkdirTemp("", "pixiv-license-bundle-check-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temporary)
		if err := writeBundle(bundle, filepath.Join(temporary, "THIRD_PARTY_LICENSES.md"), filepath.Join(temporary, "third_party", "licenses")); err != nil {
			return err
		}
		if err := compareFile(index, filepath.Join(temporary, "THIRD_PARTY_LICENSES.md")); err != nil {
			return fmt.Errorf("generated index is stale: %w", err)
		}
		if err := compareTree(licenses, filepath.Join(temporary, "third_party", "licenses")); err != nil {
			return fmt.Errorf("generated license directory is stale: %w", err)
		}
		return nil
	}
	return writeBundle(bundle, index, licenses)
}

// repositoryOutputPath 只接受 repository 内部的相对输出路径。许可证生成会替换目录，
// 因而绝不能让配置把删除/rename 操作引向仓库外或既有 symlink 的目标。
func repositoryOutputPath(repository, value, label string) (string, error) {
	if value == "" || filepath.IsAbs(value) {
		return "", fmt.Errorf("%s must be a repository-relative output path: %q", label, value)
	}
	for _, component := range strings.FieldsFunc(value, func(character rune) bool {
		return character == '/' || character == '\\'
	}) {
		if component == ".." {
			return "", fmt.Errorf("%s must be a repository-relative output path: %q", label, value)
		}
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s must be a repository-relative output path: %q", label, value)
	}
	path := filepath.Join(repository, clean)
	relative, err := filepath.Rel(repository, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s must be a repository-relative output path: %q", label, value)
	}
	current := repository
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("inspect %s output path %q: %w", label, value, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%s must be a repository-relative output path without symlinks: %q", label, value)
		}
	}
	return path, nil
}

func bundleOutputPathsOverlap(first, second string) bool {
	for _, paths := range [][2]string{{first, second}, {second, first}} {
		relative, err := filepath.Rel(paths[0], paths[1])
		if err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))) {
			return true
		}
	}
	return false
}

func packageLicenseFiles(pkg packageMetadata) ([]licenseFile, error) {
	directory := pkg.Directory
	names := map[string]bool{}
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		// 子组件可能把多份许可单列在嵌套 LICENSES 目录。
		for _, part := range strings.Split(relative, string(filepath.Separator)) {
			if isLicenseName(part) {
				names[relative] = true
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read package %s %s: %w", pkg.Name, pkg.Version, err)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("package %s %s has no LICENSE/COPYING/NOTICE files", pkg.Name, pkg.Version)
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	files := make([]licenseFile, 0, len(ordered))
	for _, name := range ordered {
		body, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			return nil, fmt.Errorf("read package %s %s license file %s: %w", pkg.Name, pkg.Version, name, err)
		}
		if len(bytes.TrimSpace(body)) == 0 {
			return nil, fmt.Errorf("package %s %s license file %s is empty", pkg.Name, pkg.Version, name)
		}
		files = append(files, licenseFile{Name: filepath.ToSlash(name), Body: normalizeLicenseText(body)})
	}
	return files, nil
}

// 归档固定 LF checkout；统一上游 CRLF 并移除末尾空行，正文内容不变。
func normalizeLicenseText(body []byte) []byte {
	return append(bytes.TrimRight(bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n")), "\r\n"), '\n')
}

func isLicenseName(name string) bool {
	upper := strings.ToUpper(name)
	if strings.HasSuffix(upper, ".GO") {
		return false
	}
	return strings.HasPrefix(upper, "LICENSE") || strings.HasPrefix(upper, "COPYING") || strings.HasPrefix(upper, "NOTICE") || strings.HasPrefix(upper, "PATENTS") || strings.HasPrefix(upper, "COPYRIGHT")
}

func writeBundle(bundle []bundledPackage, indexPath, licensesDir string) error {
	if err := os.MkdirAll(filepath.Dir(licensesDir), 0o755); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(licensesDir), ".licenses-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	for _, pkg := range bundle {
		packageDir := filepath.Join(temporary, packageDirectory(pkg.Package))
		if err := os.MkdirAll(packageDir, 0o755); err != nil {
			return err
		}
		for _, file := range pkg.Files {
			path := filepath.Join(packageDir, filepath.FromSlash(file.Name))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, file.Body, 0o644); err != nil {
				return err
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return err
	}
	indexTemporary, err := os.CreateTemp(filepath.Dir(indexPath), ".third-party-licenses-*")
	if err != nil {
		return err
	}
	indexTemporaryPath := indexTemporary.Name()
	defer os.Remove(indexTemporaryPath)
	if _, err := indexTemporary.WriteString(renderIndex(bundle, indexPath, licensesDir)); err != nil {
		_ = indexTemporary.Close()
		return err
	}
	if err := indexTemporary.Close(); err != nil {
		return err
	}
	// 先删除旧树再发布完整新树；生成过程失败时旧输出保持不变。
	if err := os.RemoveAll(licensesDir); err != nil {
		return fmt.Errorf("remove previous license tree: %w", err)
	}
	if err := os.Rename(temporary, licensesDir); err != nil {
		return fmt.Errorf("publish license tree: %w", err)
	}
	if err := os.Rename(indexTemporaryPath, indexPath); err != nil {
		return fmt.Errorf("publish license index: %w", err)
	}
	return nil
}

func renderIndex(bundle []bundledPackage, indexPath, licensesDir string) string {
	relative, err := filepath.Rel(filepath.Dir(indexPath), licensesDir)
	if err != nil {
		relative = licensesDir
	}
	var out strings.Builder
	out.WriteString("# Third-party licenses\n\n")
	out.WriteString("Generated by `go run ./scripts/build/licensebundle` from the six-target production Go module union (`CGO_ENABLED=0 go list -mod=readonly -deps -json ./cmd/pixiv`) and `third_party/attributions.json`. Release archives include this index and every license text; do not edit generated files.\n")
	out.WriteString("\n## Six-target release dependency union\n")
	for _, pkg := range bundle {
		out.WriteString("\n### " + pkg.Package.Name + " " + pkg.Package.Version + "\n\n")
		if pkg.Package.License != "" {
			out.WriteString("- SPDX: `" + pkg.Package.License + "`\n")
		}
		if pkg.Package.Repository != "" {
			out.WriteString("- Repository: " + pkg.Package.Repository + "\n")
		}
		if pkg.Package.Note != "" {
			out.WriteString("- Scope: " + pkg.Package.Note + "\n")
		}
		out.WriteString("- License texts:\n")
		for _, file := range pkg.Files {
			path := filepath.ToSlash(filepath.Join(relative, packageDirectory(pkg.Package), file.Name))
			out.WriteString("  - [" + file.Name + "](" + path + ")\n")
		}
	}
	return out.String()
}

var unsafePathPart = regexp.MustCompile(`[^A-Za-z0-9._+-]+`)

func packageDirectory(pkg packageMetadata) string {
	return unsafePathPart.ReplaceAllString(pkg.Name, "_") + "-" + unsafePathPart.ReplaceAllString(pkg.Version, "_")
}

func compareFile(actual, expected string) error {
	actualBody, err := os.ReadFile(actual)
	if err != nil {
		return err
	}
	expectedBody, err := os.ReadFile(expected)
	if err != nil {
		return err
	}
	if !bytes.Equal(actualBody, expectedBody) {
		return errors.New("content differs")
	}
	return nil
}

func compareTree(actual, expected string) error {
	actualFiles, err := treeFiles(actual)
	if err != nil {
		return err
	}
	expectedFiles, err := treeFiles(expected)
	if err != nil {
		return err
	}
	if strings.Join(actualFiles, "\n") != strings.Join(expectedFiles, "\n") {
		return fmt.Errorf("file lists differ: actual=%v expected=%v", actualFiles, expectedFiles)
	}
	for _, name := range actualFiles {
		if err := compareFile(filepath.Join(actual, name), filepath.Join(expected, name)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func treeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}
