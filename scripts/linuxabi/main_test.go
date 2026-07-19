package main

import (
	"debug/elf"
	"errors"
	"testing"
)

func TestCheckImportedSymbolsAcceptsSupportedGLIBC(t *testing.T) {
	t.Parallel()

	symbols := []elf.ImportedSymbol{
		{Name: "memcpy", Version: "GLIBC_2.14", Library: "libc.so.6"},
		{Name: "pthread_create", Version: "GLIBC_2.34", Library: "libc.so.6"},
		{Name: "fstat64", Version: "GLIBC_2.35", Library: "libc.so.6"},
		{Name: "_ZNSt8ios_base4InitC1Ev", Version: "GLIBCXX_3.4.29", Library: "libstdc++.so.6"},
	}

	if err := checkImportedSymbols(symbols); err != nil {
		t.Fatalf("checkImportedSymbols() error = %v", err)
	}
}

func TestCheckImportedSymbolsRejectsNewerGLIBC(t *testing.T) {
	t.Parallel()

	err := checkImportedSymbols([]elf.ImportedSymbol{
		{Name: "pidfd_spawnp", Version: "GLIBC_2.39", Library: "libc.so.6"},
		{Name: "pidfd_getpid", Version: "GLIBC_2.39", Library: "libc.so.6"},
	})
	if err == nil {
		t.Fatal("checkImportedSymbols() error = nil, want incompatible GLIBC error")
	}
	var compatibilityErr *glibcCompatibilityError
	if !errors.As(err, &compatibilityErr) {
		t.Fatalf("checkImportedSymbols() error type = %T, want *glibcCompatibilityError", err)
	}
	if compatibilityErr.Required != "2.39" || compatibilityErr.Maximum != "2.35" {
		t.Fatalf("compatibility error = %#v", compatibilityErr)
	}
	if got := compatibilityErr.Sources; len(got) != 2 || got[0] != "pidfd_getpid" || got[1] != "pidfd_spawnp" {
		t.Fatalf("compatibility symbols = %v", got)
	}
}

func TestCheckDynamicVersionNeedsRejectsNewerGLIBCWithoutSymbolMetadata(t *testing.T) {
	t.Parallel()

	err := checkDynamicVersionNeeds([]elf.DynamicVersionNeed{
		{
			Name: "libc.so.6",
			Needs: []elf.DynamicVersionDep{
				{Dep: "GLIBC_2.34"},
				{Dep: "GLIBC_2.39"},
			},
		},
	})
	if err == nil {
		t.Fatal("checkDynamicVersionNeeds() error = nil, want incompatible GLIBC error")
	}
	var compatibilityErr *glibcCompatibilityError
	if !errors.As(err, &compatibilityErr) {
		t.Fatalf("checkDynamicVersionNeeds() error type = %T, want *glibcCompatibilityError", err)
	}
	if compatibilityErr.Required != "2.39" || len(compatibilityErr.Sources) != 1 || compatibilityErr.Sources[0] != "libc.so.6" {
		t.Fatalf("compatibility error = %#v", compatibilityErr)
	}
}

func TestParseGLIBCVersion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input string
		want  glibcVersion
		ok    bool
	}{
		{input: "GLIBC_2.2.5", want: glibcVersion{2, 2, 5}, ok: true},
		{input: "GLIBC_2.35", want: glibcVersion{2, 35}, ok: true},
		{input: "GLIBCXX_3.4.29", ok: false},
		{input: "GLIBC_2.x", ok: false},
		{input: "", ok: false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := parseGLIBCVersion(tc.input)
			if ok != tc.ok || got.compare(tc.want) != 0 {
				t.Fatalf("parseGLIBCVersion(%q) = (%v, %v), want (%v, %v)", tc.input, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestRunRequiresBinary(t *testing.T) {
	t.Parallel()

	if err := run(nil); err == nil {
		t.Fatal("run(nil) error = nil, want usage error")
	}
}
