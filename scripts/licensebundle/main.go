// Command licensebundle 从六个 release Rust target 的锁定离线依赖图生成许可证 bundle。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var releaseTargets = []string{
	"x86_64-apple-darwin",
	"aarch64-apple-darwin",
	"x86_64-unknown-linux-gnu",
	"aarch64-unknown-linux-gnu",
	"x86_64-pc-windows-msvc",
	"aarch64-pc-windows-msvc",
}

type cargoMetadata struct {
	Packages []cargoPackage `json:"packages"`
	Resolve  struct {
		Nodes []cargoNode `json:"nodes"`
	} `json:"resolve"`
}

type cargoPackage struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Version      string  `json:"version"`
	License      string  `json:"license"`
	LicenseFile  *string `json:"license_file"`
	ManifestPath string  `json:"manifest_path"`
	Repository   string  `json:"repository"`
}

type cargoNode struct {
	ID   string         `json:"id"`
	Deps []cargoNodeDep `json:"deps"`
}

type cargoNodeDep struct {
	PackageID string `json:"pkg"`
}

type licenseFile struct {
	Name string
	Body []byte
}

type bundledPackage struct {
	Package cargoPackage
	Files   []licenseFile
}

type options struct {
	Manifest   string
	Index      string
	Licenses   string
	Check      bool
	Cargo      string
	Repository string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "license bundle:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("licensebundle", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	opts := options{}
	flags.StringVar(&opts.Manifest, "manifest", "internal/download/ugoira_rs/Cargo.toml", "Rust crate manifest")
	flags.StringVar(&opts.Index, "index", "THIRD_PARTY_LICENSES.md", "generated bundle index")
	flags.StringVar(&opts.Licenses, "licenses-dir", "third_party/licenses", "generated license text directory")
	flags.BoolVar(&opts.Check, "check", false, "verify generated files without modifying them")
	flags.StringVar(&opts.Cargo, "cargo", "cargo", "cargo executable")
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
	manifest := absoluteFrom(repository, opts.Manifest)
	index := absoluteFrom(repository, opts.Index)
	licenses := absoluteFrom(repository, opts.Licenses)
	metadata := make([][]byte, 0, len(releaseTargets))
	for _, target := range releaseTargets {
		body, err := cargoMetadataJSON(opts.Cargo, repository, manifest, target)
		if err != nil {
			return fmt.Errorf("resolve %s dependency closure: %w", target, err)
		}
		metadata = append(metadata, body)
	}
	if opts.Check {
		temporary, err := os.MkdirTemp("", "pixiv-license-bundle-check-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temporary)
		if err := generateFromTargetMetadata(metadata, manifest, filepath.Join(temporary, "THIRD_PARTY_LICENSES.md"), filepath.Join(temporary, "third_party", "licenses")); err != nil {
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
	return generateFromTargetMetadata(metadata, manifest, index, licenses)
}

func absoluteFrom(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(root, path)
}

func cargoMetadataJSON(cargo, repository, manifest, target string) ([]byte, error) {
	command := exec.Command(cargo, "metadata", "--locked", "--offline", "--format-version", "1", "--filter-platform", target, "--manifest-path", manifest)
	command.Dir = repository
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return nil, fmt.Errorf("run cargo metadata --locked --offline for %s: %w", target, err)
		}
		return nil, fmt.Errorf("run cargo metadata --locked --offline for %s: %w: %s", target, err, detail)
	}
	return stdout.Bytes(), nil
}

func generateFromTargetMetadata(rawMetadata [][]byte, rootManifest, indexPath, licensesDir string) error {
	rootManifest, err := filepath.Abs(rootManifest)
	if err != nil {
		return err
	}
	packages := map[string]cargoPackage{}
	closure := map[string]bool{}
	for _, raw := range rawMetadata {
		var metadata cargoMetadata
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return fmt.Errorf("decode cargo metadata: %w", err)
		}
		rootID, err := rootPackageID(metadata.Packages, rootManifest)
		if err != nil {
			return err
		}
		for _, pkg := range metadata.Packages {
			if previous, ok := packages[pkg.ID]; ok && previous.ManifestPath != pkg.ManifestPath {
				return fmt.Errorf("package %s resolves to conflicting manifest paths", pkg.ID)
			}
			packages[pkg.ID] = pkg
		}
		for id := range dependencyClosure(rootID, metadata.Resolve.Nodes) {
			if id != rootID {
				closure[id] = true
			}
		}
	}
	bundle := make([]bundledPackage, 0, len(closure))
	for id := range closure {
		pkg, ok := packages[id]
		if !ok {
			return fmt.Errorf("resolved dependency %s is missing package metadata", id)
		}
		files, err := packageLicenseFiles(pkg)
		if err != nil {
			return err
		}
		bundle = append(bundle, bundledPackage{Package: pkg, Files: files})
	}
	sort.Slice(bundle, func(i, j int) bool {
		if bundle[i].Package.Name == bundle[j].Package.Name {
			return bundle[i].Package.Version < bundle[j].Package.Version
		}
		return bundle[i].Package.Name < bundle[j].Package.Name
	})
	return writeBundle(bundle, indexPath, licensesDir)
}

func rootPackageID(packages []cargoPackage, rootManifest string) (string, error) {
	for _, pkg := range packages {
		manifest, err := filepath.Abs(pkg.ManifestPath)
		if err != nil {
			return "", err
		}
		if filepath.Clean(manifest) == filepath.Clean(rootManifest) {
			return pkg.ID, nil
		}
	}
	return "", fmt.Errorf("root package for manifest %s not found in cargo metadata", rootManifest)
}

func dependencyClosure(root string, nodes []cargoNode) map[string]bool {
	byID := make(map[string]cargoNode, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	seen := map[string]bool{}
	queue := []string{root}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, dependency := range byID[id].Deps {
			if !seen[dependency.PackageID] {
				queue = append(queue, dependency.PackageID)
			}
		}
	}
	return seen
}

func packageLicenseFiles(pkg cargoPackage) ([]licenseFile, error) {
	directory := filepath.Dir(pkg.ManifestPath)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read package %s %s: %w", pkg.Name, pkg.Version, err)
	}
	names := map[string]bool{}
	for _, entry := range entries {
		if entry.Type().IsRegular() && isLicenseName(entry.Name()) {
			names[entry.Name()] = true
		}
	}
	if pkg.LicenseFile != nil && *pkg.LicenseFile != "" {
		name := filepath.Clean(*pkg.LicenseFile)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("package %s %s has unsafe license_file %q", pkg.Name, pkg.Version, name)
		}
		names[name] = true
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

// normalizeLicenseText 仅移除许可证正文末尾的多余空行，避免 Git 将生成的上游文本判为
// blank-at-EOF；其余字节（包括正文内的换行风格）保持不变。
func normalizeLicenseText(body []byte) []byte {
	return append(bytes.TrimRight(body, "\r\n"), '\n')
}

func isLicenseName(name string) bool {
	upper := strings.ToUpper(name)
	return strings.HasPrefix(upper, "LICENSE") || strings.HasPrefix(upper, "COPYING") || strings.HasPrefix(upper, "NOTICE")
}

func writeBundle(bundle []bundledPackage, indexPath, licensesDir string) error {
	parent := filepath.Dir(licensesDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(parent, ".licenses-*")
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
		indexTemporary.Close()
		return err
	}
	if err := indexTemporary.Close(); err != nil {
		return err
	}
	if err := os.RemoveAll(licensesDir); err != nil {
		return err
	}
	if err := os.Rename(temporary, licensesDir); err != nil {
		return err
	}
	if err := os.Rename(indexTemporaryPath, indexPath); err != nil {
		return err
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
	out.WriteString("本文件由 `go run ./scripts/licensebundle` 根据 `Cargo.lock` 与六个 Rust release target 的 `cargo metadata --locked --offline` 并集生成。发布包必须携带本索引和完整许可证正文目录；不要手工编辑。\n")
	out.WriteString("\n## Six-target release dependency union\n")
	for _, pkg := range bundle {
		out.WriteString("\n### " + pkg.Package.Name + " " + pkg.Package.Version + "\n\n")
		if pkg.Package.License != "" {
			out.WriteString("- SPDX: `" + pkg.Package.License + "`\n")
		}
		if pkg.Package.Repository != "" {
			out.WriteString("- Repository: " + pkg.Package.Repository + "\n")
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

func packageDirectory(pkg cargoPackage) string {
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
