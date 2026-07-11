package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	fakeCargo := filepath.Join(dir, "fake-cargo")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
target=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --filter-platform) target=$2; shift 2 ;;
    *) shift ;;
  esac
done
printf '%%s\n' "$target" >> %s
printf '%%s' %s
`, shellQuote(record), shellQuote(string(metadata)))
	if err := os.WriteFile(fakeCargo, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := generate(options{
		Repository: dir,
		Manifest:   root.manifest,
		Index:      filepath.Join(dir, "THIRD_PARTY_LICENSES.md"),
		Licenses:   filepath.Join(dir, "third_party", "licenses"),
		Cargo:      fakeCargo,
	}); err != nil {
		t.Fatalf("generate returned error: %v", err)
	}
	body, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(body))
	want := append([]string(nil), releaseTargets...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("cargo metadata targets = %v, want %v", got, want)
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
