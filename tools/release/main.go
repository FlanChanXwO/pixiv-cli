// Command release provides small release-material operations shared by CI.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type registry struct {
	Platforms []platform
}

type platform struct {
	GOOS         string
	GOARCH       string
	Capabilities []string
}

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("expected verify-source, verify-published-release, verify-handoff, verify-handoff-identity, write-handoff, verify-handoff-set, verify-artifact-set, verify-container-set, or checksums"))
	}
	switch os.Args[1] {
	case "verify-source":
		verifySourceCommand(os.Args[2:])
	case "verify-published-release":
		verifyPublishedReleaseCommand(os.Args[2:])
	case "verify-handoff":
		verifyHandoffCommand(os.Args[2:])
	case "write-handoff":
		writeHandoffCommand(os.Args[2:])
	case "verify-handoff-set":
		verifyHandoffSetCommand(os.Args[2:])
	case "verify-handoff-identity":
		verifyHandoffIdentityCommand(os.Args[2:])
	case "verify-artifact-set":
		verifyArtifactSet(os.Args[2:])
	case "verify-container-set":
		verifyContainerSet(os.Args[2:])
	case "checksums":
		writeChecksums(os.Args[2:])
	default:
		fatal(fmt.Errorf("unknown release operation %q", os.Args[1]))
	}
}

func verifyContainerSet(args []string) {
	set := flag.NewFlagSet("verify-container-set", flag.ExitOnError)
	dir := set.String("dir", "containers", "container artifact directory")
	registryPath := set.String("registry", "ci/platforms.json", "platform registry")
	_ = set.Parse(args)
	expected, err := expectedContainerArtifacts(*registryPath)
	if err != nil {
		fatal(err)
	}
	entries, err := os.ReadDir(*dir)
	if err != nil {
		fatal(err)
	}
	var actual []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "pixiv-cli-") && strings.HasSuffix(entry.Name(), ".tar") {
			actual = append(actual, entry.Name())
		}
	}
	sort.Strings(actual)
	if strings.Join(expected, "\n") != strings.Join(actual, "\n") {
		fatal(fmt.Errorf("container artifact set mismatch\nexpected:\n%s\nactual:\n%s",
			strings.Join(expected, "\n"), strings.Join(actual, "\n")))
	}
}

func verifyArtifactSet(args []string) {
	set := flag.NewFlagSet("verify-artifact-set", flag.ExitOnError)
	dir := set.String("dir", "dist", "release artifact directory")
	version := set.String("version", "", "release version without v")
	registryPath := set.String("registry", "ci/platforms.json", "platform registry")
	_ = set.Parse(args)
	expected, err := expectedArtifacts(*registryPath, *version)
	if err != nil {
		fatal(err)
	}
	actual, err := releaseArchives(*dir, *version)
	if err != nil {
		fatal(err)
	}
	if strings.Join(expected, "\n") != strings.Join(actual, "\n") {
		fatal(fmt.Errorf("release artifact set mismatch\nexpected:\n%s\nactual:\n%s",
			strings.Join(expected, "\n"), strings.Join(actual, "\n")))
	}
}

func writeChecksums(args []string) {
	set := flag.NewFlagSet("checksums", flag.ExitOnError)
	dir := set.String("dir", "dist", "release artifact directory")
	version := set.String("version", "", "release version without v")
	registryPath := set.String("registry", "ci/platforms.json", "platform registry")
	output := set.String("output", "", "checksums output path")
	_ = set.Parse(args)
	if *output == "" {
		fatal(errors.New("output is required"))
	}
	expected, err := expectedArtifacts(*registryPath, *version)
	if err != nil {
		fatal(err)
	}
	actual, err := releaseArchives(*dir, *version)
	if err != nil {
		fatal(err)
	}
	if strings.Join(expected, "\n") != strings.Join(actual, "\n") {
		fatal(errors.New("refusing to checksum an incomplete or unexpected artifact set"))
	}
	file, err := os.Create(*output)
	if err != nil {
		fatal(err)
	}
	writer := bufio.NewWriter(file)
	for _, name := range expected {
		path := filepath.Join(*dir, name)
		input, err := os.Open(path)
		if err != nil {
			_ = file.Close()
			fatal(err)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, input)
		closeErr := input.Close()
		if copyErr != nil {
			_ = file.Close()
			fatal(copyErr)
		}
		if closeErr != nil {
			_ = file.Close()
			fatal(closeErr)
		}
		fmt.Fprintf(writer, "%s  %s\n", hex.EncodeToString(hash.Sum(nil)), name)
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		fatal(err)
	}
	if err := file.Close(); err != nil {
		fatal(err)
	}
}

func expectedArtifacts(registryPath, version string) ([]string, error) {
	if strings.TrimSpace(version) == "" {
		return nil, errors.New("version is required")
	}
	body, err := os.ReadFile(registryPath)
	if err != nil {
		return nil, err
	}
	var cfg registry
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, err
	}
	var names []string
	seen := map[string]struct{}{}
	for _, item := range cfg.Platforms {
		if !contains(item.Capabilities, "release") {
			continue
		}
		if item.GOOS == "" || item.GOARCH == "" {
			return nil, errors.New("release platform has empty goos/goarch")
		}
		key := item.GOOS + "/" + item.GOARCH
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate release platform %s", key)
		}
		seen[key] = struct{}{}
		extension := "tar.gz"
		if item.GOOS == "windows" {
			extension = "zip"
		}
		names = append(names, fmt.Sprintf("pixiv-cli_%s_%s_%s.%s", version, item.GOOS, item.GOARCH, extension))
	}
	if len(names) == 0 {
		return nil, errors.New("registry has no release platforms")
	}
	sort.Strings(names)
	return names, nil
}

func expectedContainerArtifacts(registryPath string) ([]string, error) {
	body, err := os.ReadFile(registryPath)
	if err != nil {
		return nil, err
	}
	var cfg registry
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, err
	}
	var names []string
	seen := map[string]struct{}{}
	for _, item := range cfg.Platforms {
		if !contains(item.Capabilities, "container") {
			continue
		}
		key := item.GOOS + "/" + item.GOARCH
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate container platform %s", key)
		}
		seen[key] = struct{}{}
		names = append(names, fmt.Sprintf("pixiv-cli-%s-%s.tar", item.GOOS, item.GOARCH))
	}
	if len(names) == 0 {
		return nil, errors.New("registry has no container platforms")
	}
	sort.Strings(names)
	return names, nil
}

func releaseArchives(dir, version string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	prefix := "pixiv-cli_" + version + "_"
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".tar.gz") || strings.HasSuffix(entry.Name(), ".zip") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
