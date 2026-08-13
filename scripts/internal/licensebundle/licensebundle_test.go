package licensebundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestGenerateFromTargetMetadataUnionsPlatformDependencyClosures(t *testing.T) {
	dir := t.TempDir()
	root := makeTestPackage(t, dir, "ugoira_rs", "0.1.0", "")
	darwinOnly := makeTestPackage(t, dir, "darwin-only", "1.0.0", "LICENSE-MIT")
	windowsOnly := makeTestPackage(t, dir, "windows-only", "2.0.0", "LICENSE-MIT")
	index := filepath.Join(dir, "THIRD_PARTY_LICENSES.md")
	licenses := filepath.Join(dir, "third_party", "licenses")

	err := generateFromTargetMetadata([][]byte{
		makeTargetMetadata(t, root, darwinOnly),
		makeTargetMetadata(t, root, windowsOnly),
	}, root.manifest, index, licenses)
	if err != nil {
		t.Fatalf("generateFromTargetMetadata returned error: %v", err)
	}
	for _, path := range []string{
		"darwin-only-1.0.0/LICENSE-MIT",
		"windows-only-2.0.0/LICENSE-MIT",
	} {
		if _, err := os.Stat(filepath.Join(licenses, path)); err != nil {
			t.Fatalf("missing six-target-union license %q: %v", path, err)
		}
	}
	body, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Six-target release dependency union", "darwin-only 1.0.0", "windows-only 2.0.0"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("index does not contain %q:\n%s", want, body)
		}
	}
}

func TestPackageLicenseFilesRejectsTraversalLicenseFile(t *testing.T) {
	dir := t.TempDir()
	pkg := makeTestPackage(t, dir, "unsafe", "1.0.0", "LICENSE-MIT")
	unsafe := "../outside"
	_, err := packageLicenseFiles(cargoPackage{Name: pkg.name, Version: pkg.version, ManifestPath: pkg.manifest, LicenseFile: &unsafe})
	if err == nil || !strings.Contains(err.Error(), "unsafe license_file") {
		t.Fatalf("packageLicenseFiles error = %v, want unsafe license_file", err)
	}
}

func TestNormalizeLicenseTextPreservesContentAndRemovesOnlyTerminalBlankLines(t *testing.T) {
	got := normalizeLicenseText([]byte("first line\r\nsecond line\n\n\n"))
	if want := "first line\r\nsecond line\n"; string(got) != want {
		t.Fatalf("normalizeLicenseText = %q, want %q", got, want)
	}
}

func TestGenerateQueriesAllSixReleaseTargetsOffline(t *testing.T) {
	dir := t.TempDir()
	root := makeTestPackage(t, dir, "ugoira_rs", "0.1.0", "")
	dependency := makeTestPackage(t, dir, "dependency", "1.0.0", "LICENSE-MIT")
	metadata := makeTargetMetadata(t, root, dependency)
	record := filepath.Join(dir, "targets.txt")
	fakeCargo := buildFakeCargo(t, dir, record, root.manifest, metadata)
	if err := generate(options{
		Repository: dir,
		Manifest:   root.manifest,
		Index:      "THIRD_PARTY_LICENSES.md",
		Licenses:   "third_party/licenses",
		Cargo:      fakeCargo,
	}); err != nil {
		t.Fatalf("generate returned error: %v", err)
	}
	body, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			t.Fatalf("cargo invocation = %q, want working directory and target", line)
		}
		if parts[0] != filepath.Dir(root.manifest) {
			t.Fatalf("cargo working directory = %q, want crate directory %q", parts[0], filepath.Dir(root.manifest))
		}
		got = append(got, parts[1])
	}
	want := append([]string(nil), releaseTargets...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("cargo metadata targets = %v, want %v", got, want)
	}
}

// buildFakeCargo 编译真实的临时可执行文件，而非依赖 POSIX shell 脚本。
// Windows 的 exec.Command 只会按 PATHEXT 查找可执行文件，不能把无扩展名
// 的 shell fixture 当作 cargo；该 helper 同时在各平台验证相同的调用契约。
func buildFakeCargo(t *testing.T, dir, record, manifest string, metadata []byte) string {
	t.Helper()

	output := filepath.Join(dir, "fake-cargo")
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	sourcePath := filepath.Join(dir, "fake-cargo.go")
	source := fmt.Sprintf(`package main

import (
	"fmt"
	"os"
)

func main() {
	var target, format, gotManifest string
	var locked, offline bool
	for arguments := os.Args[1:]; len(arguments) > 0; {
		switch arguments[0] {
		case "--filter-platform":
			if len(arguments) < 2 { os.Exit(2) }
			target, arguments = arguments[1], arguments[2:]
		case "--locked":
			locked, arguments = true, arguments[1:]
		case "--offline":
			offline, arguments = true, arguments[1:]
		case "--format-version":
			if len(arguments) < 2 { os.Exit(2) }
			format, arguments = arguments[1], arguments[2:]
		case "--manifest-path":
			if len(arguments) < 2 { os.Exit(2) }
			gotManifest, arguments = arguments[1], arguments[2:]
		default:
			arguments = arguments[1:]
		}
	}
	if !locked || !offline || format != "1" || gotManifest != %q { os.Exit(3) }
	workingDirectory, err := os.Getwd()
	if err != nil { os.Exit(4) }
	record, err := os.OpenFile(%q, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil { os.Exit(5) }
	defer record.Close()
	if _, err := fmt.Fprintf(record, "%%s\t%%s\n", workingDirectory, target); err != nil { os.Exit(6) }
	fmt.Print(%q)
}
`, manifest, record, string(metadata))
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "build", "-o", output, sourcePath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build cross-platform fake cargo: %v\n%s", err, output)
	}
	return output
}

func TestGenerateRejectsOutputPathsOutsideRepositoryBeforeCargo(t *testing.T) {
	base := t.TempDir()
	repository := filepath.Join(base, "repository")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "licenses"), 0o755); err != nil {
		t.Fatal(err)
	}
	outsideIndex := filepath.Join(outside, "THIRD_PARTY_LICENSES.md")
	if err := os.WriteFile(outsideIndex, []byte("outside index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideLicense := filepath.Join(outside, "licenses", "keep")
	if err := os.WriteFile(outsideLicense, []byte("outside license\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repository, "outside-link")); err != nil {
		t.Skipf("test environment cannot create a symlink: %v", err)
	}
	for name, opts := range map[string]options{
		"absolute index": {
			Repository: repository,
			Manifest:   "ignored/Cargo.toml",
			Index:      outsideIndex,
			Licenses:   "third_party/licenses",
			Cargo:      "missing-cargo-must-not-run",
		},
		"escaped licenses": {
			Repository: repository,
			Manifest:   "ignored/Cargo.toml",
			Index:      "THIRD_PARTY_LICENSES.md",
			Licenses:   "../outside/licenses",
			Cargo:      "missing-cargo-must-not-run",
		},
		"lexical traversal": {
			Repository: repository,
			Manifest:   "ignored/Cargo.toml",
			Index:      "generated/../THIRD_PARTY_LICENSES.md",
			Licenses:   "third_party/licenses",
			Cargo:      "missing-cargo-must-not-run",
		},
		"overlapping outputs": {
			Repository: repository,
			Manifest:   "ignored/Cargo.toml",
			Index:      "third_party/licenses/index.md",
			Licenses:   "third_party/licenses",
			Cargo:      "missing-cargo-must-not-run",
		},
		"symlinked index": {
			Repository: repository,
			Manifest:   "ignored/Cargo.toml",
			Index:      "outside-link/THIRD_PARTY_LICENSES.md",
			Licenses:   "third_party/licenses",
			Cargo:      "missing-cargo-must-not-run",
		},
		"symlinked licenses": {
			Repository: repository,
			Manifest:   "ignored/Cargo.toml",
			Index:      "THIRD_PARTY_LICENSES.md",
			Licenses:   "outside-link/licenses",
			Cargo:      "missing-cargo-must-not-run",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := generate(opts)
			if err == nil || (!strings.Contains(err.Error(), "repository-relative") && !strings.Contains(err.Error(), "must not overlap")) {
				t.Fatalf("generate error = %v, want repository-contained output path rejection", err)
			}
			if body, err := os.ReadFile(outsideIndex); err != nil || string(body) != "outside index\n" {
				t.Fatalf("outside index changed: body=%q err=%v", body, err)
			}
			if body, err := os.ReadFile(outsideLicense); err != nil || string(body) != "outside license\n" {
				t.Fatalf("outside license sentinel changed: body=%q err=%v", body, err)
			}
		})
	}
}

func TestWriteBundleRestoresPreviousIndexAndTreeWhenIndexPublishFails(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "THIRD_PARTY_LICENSES.md")
	licenses := filepath.Join(dir, "third_party", "licenses")
	if err := os.MkdirAll(licenses, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, []byte("old index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldLicense := filepath.Join(licenses, "old", "LICENSE")
	if err := os.MkdirAll(filepath.Dir(oldLicense), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldLicense, []byte("old license\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected index publication failure")
	ops := defaultBundleFileOps()
	ops.rename = func(source, destination string) error {
		if destination == index && strings.HasPrefix(filepath.Base(source), ".third-party-licenses-") {
			return injected
		}
		return os.Rename(source, destination)
	}
	err := writeBundleWithFileOps([]bundledPackage{{
		Package: cargoPackage{Name: "new", Version: "1.0.0"},
		Files:   []licenseFile{{Name: "LICENSE", Body: []byte("new license\n")}},
	}}, index, licenses, ops)
	if !errors.Is(err, injected) {
		t.Fatalf("writeBundleWithFileOps error = %v, want injected publication failure", err)
	}
	if body, err := os.ReadFile(index); err != nil || string(body) != "old index\n" {
		t.Fatalf("index was not restored: body=%q err=%v", body, err)
	}
	if body, err := os.ReadFile(oldLicense); err != nil || string(body) != "old license\n" {
		t.Fatalf("license tree was not restored: body=%q err=%v", body, err)
	}
	if _, err := os.Stat(filepath.Join(licenses, "new", "LICENSE")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new license tree remained after failed publication: %v", err)
	}
}

type testPackage struct {
	id       string
	name     string
	version  string
	manifest string
}

func makeTestPackage(t *testing.T, root, name, version, licenseFile string) testPackage {
	t.Helper()
	directory := filepath.Join(root, name+"-"+version)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(directory, "Cargo.toml")
	if err := os.WriteFile(manifest, []byte("[package]\nname = \""+name+"\"\nversion = \""+version+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if licenseFile != "" {
		if err := os.WriteFile(filepath.Join(directory, licenseFile), []byte("Permission is hereby granted, free of charge, to any person obtaining a copy.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return testPackage{id: name + " " + version, name: name, version: version, manifest: manifest}
}

func makeTargetMetadata(t *testing.T, root, dependency testPackage) []byte {
	t.Helper()
	metadata := map[string]any{
		"packages": []any{
			map[string]any{"id": root.id, "name": root.name, "version": root.version, "manifest_path": root.manifest},
			map[string]any{"id": dependency.id, "name": dependency.name, "version": dependency.version, "license": "MIT", "manifest_path": dependency.manifest},
		},
		"resolve": map[string]any{"nodes": []any{
			map[string]any{"id": root.id, "deps": []any{map[string]any{"pkg": dependency.id}}},
			map[string]any{"id": dependency.id, "deps": []any{}},
		}},
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
