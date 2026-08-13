// Package licensebundle 从六个 release Rust target 的锁定离线依赖图生成许可证 bundle。
package licensebundle

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

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/releasecontract"
)

var releaseTargets = func() []string {
	targets := releasecontract.FixedTargets()
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.RustTarget())
	}
	return names
}()

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

// Run 是 scripts/licensebundle 的入口 owner：解析参数并委托给许可证生成逻辑。
func Run(args []string) error {
	return run(args)
}

func run(args []string) error {
	flags := flag.NewFlagSet("licensebundle", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	opts := options{}
	flags.StringVar(&opts.Manifest, "manifest", "internal/media/ugoira/rust/Cargo.toml", "Rust crate manifest")
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

func cargoMetadataJSON(cargo, repository, manifest, target string) ([]byte, error) {
	command := exec.Command(cargo, "metadata", "--locked", "--offline", "--format-version", "1", "--filter-platform", target, "--manifest-path", manifest)
	// Cargo 仅从 command.Dir 及其父目录读取 .cargo/config.toml；必须从 crate 目录启动，
	// 才能让六目标许可证元数据与 staticlib 构建使用同一份仓库内 vendor 闭包。
	command.Dir = filepath.Dir(manifest)
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

type bundleFileOps struct {
	rename    func(string, string) error
	remove    func(string) error
	removeAll func(string) error
	lstat     func(string) (os.FileInfo, error)
}

func defaultBundleFileOps() bundleFileOps {
	return bundleFileOps{
		rename:    os.Rename,
		remove:    os.Remove,
		removeAll: os.RemoveAll,
		lstat:     os.Lstat,
	}
}

func writeBundle(bundle []bundledPackage, indexPath, licensesDir string) error {
	return writeBundleWithFileOps(bundle, indexPath, licensesDir, defaultBundleFileOps())
}

func writeBundleWithFileOps(bundle []bundledPackage, indexPath, licensesDir string, ops bundleFileOps) error {
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
	return publishBundle(temporary, indexTemporaryPath, indexPath, licensesDir, ops)
}

// publishBundle 将完整暂存 tree 和 index 作为一个可回滚的发布单元提交。旧 bundle 先被
// 同目录 rename 到 backup；任一步失败都会恢复旧 index 与完整 license tree，而不是先删旧树。
func publishBundle(temporaryTree, temporaryIndex, indexPath, licensesDir string, ops bundleFileOps) error {
	licensesExisted, err := bundlePathExists(ops, licensesDir)
	if err != nil {
		return fmt.Errorf("inspect existing license tree: %w", err)
	}
	indexExisted, err := bundlePathExists(ops, indexPath)
	if err != nil {
		return fmt.Errorf("inspect existing license index: %w", err)
	}
	licensesBackup := ""
	if licensesExisted {
		licensesBackup, err = reserveBundleBackupPath(licensesDir)
		if err != nil {
			return err
		}
	}
	indexBackup := ""
	if indexExisted {
		indexBackup, err = reserveBundleBackupPath(indexPath)
		if err != nil {
			return err
		}
	}

	licensesBackedUp := false
	licensesPublished := false
	indexBackedUp := false
	indexPublished := false
	rollback := func(cause error) error {
		var rollbackErrors []error
		if indexBackedUp {
			if indexPublished {
				if err := removeBundleFileIfPresent(ops, indexPath); err != nil {
					rollbackErrors = append(rollbackErrors, fmt.Errorf("remove newly published index: %w", err))
				}
			}
			if err := ops.rename(indexBackup, indexPath); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous index: %w", err))
			}
		} else if indexPublished {
			if err := removeBundleFileIfPresent(ops, indexPath); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove newly published index: %w", err))
			}
		}
		if licensesBackedUp {
			if licensesPublished {
				if err := removeBundleTreeIfPresent(ops, licensesDir); err != nil {
					rollbackErrors = append(rollbackErrors, fmt.Errorf("remove newly published license tree: %w", err))
				}
			}
			if err := ops.rename(licensesBackup, licensesDir); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous license tree: %w", err))
			}
		} else if licensesPublished {
			if err := removeBundleTreeIfPresent(ops, licensesDir); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove newly published license tree: %w", err))
			}
		}
		return errors.Join(append([]error{cause}, rollbackErrors...)...)
	}

	if licensesExisted {
		if err := ops.rename(licensesDir, licensesBackup); err != nil {
			return fmt.Errorf("backup previous license tree: %w", err)
		}
		licensesBackedUp = true
	}
	if err := ops.rename(temporaryTree, licensesDir); err != nil {
		return rollback(fmt.Errorf("publish license tree: %w", err))
	}
	licensesPublished = true
	if indexExisted {
		if err := ops.rename(indexPath, indexBackup); err != nil {
			return rollback(fmt.Errorf("backup previous index: %w", err))
		}
		indexBackedUp = true
	}
	if err := ops.rename(temporaryIndex, indexPath); err != nil {
		return rollback(fmt.Errorf("publish license index: %w", err))
	}
	indexPublished = true

	var cleanupErrors []error
	if licensesBackedUp {
		if err := ops.removeAll(licensesBackup); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove previous license tree backup: %w", err))
		}
	}
	if indexBackedUp {
		if err := ops.remove(indexBackup); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove previous index backup: %w", err))
		}
	}
	return errors.Join(cleanupErrors...)
}

func bundlePathExists(ops bundleFileOps, path string) (bool, error) {
	_, err := ops.lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func reserveBundleBackupPath(path string) (string, error) {
	reservation, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".backup-*")
	if err != nil {
		return "", fmt.Errorf("reserve backup path for %s: %w", path, err)
	}
	backup := reservation.Name()
	if err := reservation.Close(); err != nil {
		return "", fmt.Errorf("close backup path reservation for %s: %w", path, err)
	}
	if err := os.Remove(backup); err != nil {
		return "", fmt.Errorf("release backup path reservation for %s: %w", path, err)
	}
	return backup, nil
}

func removeBundleFileIfPresent(ops bundleFileOps, path string) error {
	err := ops.remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func removeBundleTreeIfPresent(ops bundleFileOps, path string) error {
	err := ops.removeAll(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
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
