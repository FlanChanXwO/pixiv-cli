// Command platformmatrix validates the shared CI platform registry and emits
// GitHub Actions matrix JSON for one capability.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

type registry struct {
	Platforms []platform
}

type platform struct {
	GOOS          string
	GOARCH        string
	Runner        string
	RustTarget    string
	RustToolchain string
	CC            string
	Capabilities  []string
}

func (p *platform) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	fields := map[string]any{
		"goos": &p.GOOS, "goarch": &p.GOARCH, "runner": &p.Runner,
		"rust_target": &p.RustTarget, "rust_toolchain": &p.RustToolchain,
		"cc": &p.CC, "capabilities": &p.Capabilities,
	}
	for name, target := range fields {
		value, ok := raw[name]
		if !ok {
			continue
		}
		if err := json.Unmarshal(value, target); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		delete(raw, name)
	}
	if len(raw) != 0 {
		for name := range raw {
			return fmt.Errorf("unknown platform field %q", name)
		}
	}
	return nil
}

type matrix struct {
	Include []map[string]any `json:"include"`
}

var allowedCapabilities = map[string]struct{}{
	"container": {}, "homebrew": {}, "native-evidence": {}, "race": {}, "release": {}, "smoke": {}, "verification": {},
}

func main() {
	registryPath := flag.String("registry", "ci/platforms.json", "platform registry path")
	capability := flag.String("capability", "", "required capability")
	githubOutput := flag.String("github-output", "", "GitHub Actions output file")
	flag.Parse()
	result, err := resolve(*registryPath, *capability)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve platform matrix: %v\n", err)
		os.Exit(1)
	}
	body, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode platform matrix: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(body))
	if *githubOutput != "" {
		file, err := os.OpenFile(*githubOutput, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open GitHub output: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		if _, err := fmt.Fprintf(file, "matrix=%s\n", body); err != nil {
			fmt.Fprintf(os.Stderr, "write GitHub output: %v\n", err)
			os.Exit(1)
		}
	}
}

func resolve(path, capability string) (matrix, error) {
	if _, ok := allowedCapabilities[capability]; !ok {
		return matrix{}, fmt.Errorf("unknown capability %q", capability)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return matrix{}, err
	}
	var cfg registry
	if err := json.Unmarshal(body, &cfg); err != nil {
		return matrix{}, err
	}
	if len(cfg.Platforms) == 0 {
		return matrix{}, errors.New("platform registry is empty")
	}
	seen := map[string]struct{}{}
	var include []map[string]any
	for index, item := range cfg.Platforms {
		if err := validatePlatform(item); err != nil {
			return matrix{}, fmt.Errorf("platform %d: %w", index+1, err)
		}
		key := item.GOOS + "/" + item.GOARCH
		if _, ok := seen[key]; ok {
			return matrix{}, fmt.Errorf("duplicate platform %s", key)
		}
		seen[key] = struct{}{}
		if hasCapability(item.Capabilities, capability) {
			include = append(include, map[string]any{
				"goos": item.GOOS, "goarch": item.GOARCH, "runner": item.Runner,
				"rust_target": item.RustTarget, "rust_toolchain": item.RustToolchain,
				"cc": item.CC, "capabilities": item.Capabilities,
				"artifact": item.GOOS + "-" + item.GOARCH,
			})
		}
	}
	if len(include) == 0 {
		return matrix{}, fmt.Errorf("no platform provides capability %q", capability)
	}
	return matrix{Include: include}, nil
}

func validatePlatform(item platform) error {
	if item.GOOS == "" || strings.ContainsAny(item.GOOS, "/\\") {
		return errors.New("goos must be a simple non-empty name")
	}
	if item.GOARCH == "" || strings.ContainsAny(item.GOARCH, "/\\") {
		return errors.New("goarch must be a simple non-empty name")
	}
	if item.Runner == "" || item.RustTarget == "" || item.RustToolchain == "" || item.CC == "" {
		return errors.New("runner, Rust target/toolchain, and cc are required")
	}
	seen := map[string]struct{}{}
	for _, capability := range item.Capabilities {
		if _, ok := allowedCapabilities[capability]; !ok {
			return fmt.Errorf("unknown capability %q", capability)
		}
		if _, ok := seen[capability]; ok {
			return fmt.Errorf("duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
	}
	if hasCapability(item.Capabilities, "container") && item.GOOS != "linux" {
		return errors.New("container capability currently requires linux")
	}
	if hasCapability(item.Capabilities, "homebrew") && item.GOOS == "windows" {
		return errors.New("homebrew capability is not supported on windows")
	}
	return nil
}

func hasCapability(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
