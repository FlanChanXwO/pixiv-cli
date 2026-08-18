// Package linuxabi 验证已打包 Linux executable 是否保持公开的 glibc 基线。
package linuxabi

import (
	"debug/elf"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Ubuntu 22.04 的 glibc 2.35 是当前 Linux release 的明确构建基线；锁定它可覆盖
// Debian 12 等更新发行版，并防止 hosted runner 升级后把更高版本符号静默带入资产。
const maximumSupportedGLIBC = "GLIBC_2.35"

type glibcVersion []int

func (v glibcVersion) compare(other glibcVersion) int {
	length := len(v)
	if len(other) > length {
		length = len(other)
	}
	for index := 0; index < length; index++ {
		left, right := 0, 0
		if index < len(v) {
			left = v[index]
		}
		if index < len(other) {
			right = other[index]
		}
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
	}
	return 0
}

func (v glibcVersion) String() string {
	parts := make([]string, len(v))
	for index, component := range v {
		parts[index] = strconv.Itoa(component)
	}
	return strings.Join(parts, ".")
}

type glibcCompatibilityError struct {
	Required string
	Maximum  string
	Sources  []string
}

func (e *glibcCompatibilityError) Error() string {
	return fmt.Sprintf("Linux binary requires GLIBC_%s via %s; release maximum is GLIBC_%s", e.Required, strings.Join(e.Sources, ", "), e.Maximum)
}

func parseGLIBCVersion(raw string) (glibcVersion, bool) {
	const prefix = "GLIBC_"
	if !strings.HasPrefix(raw, prefix) {
		return nil, false
	}
	components := strings.Split(strings.TrimPrefix(raw, prefix), ".")
	if len(components) < 2 {
		return nil, false
	}
	version := make(glibcVersion, len(components))
	for index, component := range components {
		if component == "" {
			return nil, false
		}
		value, err := strconv.Atoi(component)
		if err != nil || value < 0 {
			return nil, false
		}
		version[index] = value
	}
	return version, true
}

func checkImportedSymbols(symbols []elf.ImportedSymbol) error {
	maximum, ok := parseGLIBCVersion(maximumSupportedGLIBC)
	if !ok {
		return fmt.Errorf("invalid release GLIBC baseline %q", maximumSupportedGLIBC)
	}

	var newest glibcVersion
	newestSymbols := map[string]struct{}{}
	for _, symbol := range symbols {
		required, ok := parseGLIBCVersion(symbol.Version)
		if !ok || required.compare(maximum) <= 0 {
			continue
		}
		switch comparison := required.compare(newest); {
		case newest == nil || comparison > 0:
			newest = required
			newestSymbols = map[string]struct{}{symbol.Name: {}}
		case comparison == 0:
			newestSymbols[symbol.Name] = struct{}{}
		}
	}
	if newest == nil {
		return nil
	}

	symbolNames := make([]string, 0, len(newestSymbols))
	for name := range newestSymbols {
		symbolNames = append(symbolNames, name)
	}
	sort.Strings(symbolNames)
	return &glibcCompatibilityError{
		Required: newest.String(),
		Maximum:  maximum.String(),
		Sources:  symbolNames,
	}
}

func checkDynamicVersionNeeds(needs []elf.DynamicVersionNeed) error {
	requirements := make([]elf.ImportedSymbol, 0)
	for _, library := range needs {
		for _, dependency := range library.Needs {
			requirements = append(requirements, elf.ImportedSymbol{
				Name:    library.Name,
				Version: dependency.Dep,
				Library: library.Name,
			})
		}
	}
	return checkImportedSymbols(requirements)
}

func verifyBinary(path string) error {
	file, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("open Linux ELF %q: %w", path, err)
	}
	defer file.Close()

	symbols, err := file.ImportedSymbols()
	if err != nil && !errors.Is(err, elf.ErrNoSymbols) {
		return fmt.Errorf("read imported symbols from %q: %w", path, err)
	}
	if err := checkImportedSymbols(symbols); err != nil {
		return fmt.Errorf("verify %q: %w", path, err)
	}
	// Go 的 ImportedSymbols 在部分 Go+cgo 产物上不会恢复弱符号的版本；
	// SHT_GNU_verneed 是 loader 实际执行的完整 ABI 契约，必须再独立检查。
	if file.SectionByType(elf.SHT_GNU_VERNEED) != nil {
		needs, err := file.DynamicVersionNeeds()
		if err != nil {
			return fmt.Errorf("read dynamic version requirements from %q: %w", path, err)
		}
		if err := checkDynamicVersionNeeds(needs); err != nil {
			return fmt.Errorf("verify %q: %w", path, err)
		}
	}
	return nil
}

func run(args []string) error {
	flags := flag.NewFlagSet("linuxabi", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	binary := flags.String("binary", "", "Linux ELF executable to verify")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(*binary) == "" {
		return errors.New("--binary is required")
	}
	return verifyBinary(*binary)
}

// Run 是 scripts/cmd/linuxabi 的入口 owner：解析参数并委托给 ELF 校验逻辑。
func Run(args []string) error {
	return run(args)
}
