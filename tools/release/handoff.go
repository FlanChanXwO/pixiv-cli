package main

import (
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
	"strconv"
	"strings"
)

// releaseHandoff 是 Release workflow 产出的不可变交接清单（方案 §15）。
//
// 它是一次成功 Release run 的身份 + 产物事实快照：downstream publisher 只读取
// 其中记录的 identity 与 checksum，不再自行构建、不再自行推断 tag 或版本。
type releaseHandoff struct {
	Repository          string            `json:"repository"`
	ReleaseRunID        int64             `json:"release_run_id"`
	ReleaseRunAttempt   int               `json:"release_run_attempt,omitempty"`
	Workflow            string            `json:"workflow"`
	Tag                 string            `json:"tag"`
	CommitSHA           string            `json:"commit_sha"`
	RunHeadSHA          string            `json:"run_head_sha"`
	Version             string            `json:"version"`
	ProductionArtifacts []handoffArtifact `json:"production_artifacts"`
	ContainerArtifacts  []handoffArtifact `json:"container_artifacts"`
	Checksums           string            `json:"checksums_sha256"`
}

// handoffArtifact 记录单个产物的名字、字节数与内容校验和。checksum 是 publisher
// 复用同一批 bytes 的唯一依据。
type handoffArtifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type releaseHandoffInput struct {
	Repository   string
	RunID        int64
	RunAttempt   int
	Workflow     string
	Tag          string
	CommitSHA    string
	RunHeadSHA   string
	Version      string
	DistDir      string
	ContainerDir string
	Output       string
}

// writeReleaseHandoff 在 Release preparation 阶段一次性固化成批产物的事实。
// 它只记录已经存在并通过校验的产物，绝不做任何构建。
func writeReleaseHandoff(input releaseHandoffInput) (releaseHandoff, error) {
	handoff := releaseHandoff{
		Repository:        input.Repository,
		ReleaseRunID:      input.RunID,
		ReleaseRunAttempt: input.RunAttempt,
		Workflow:          input.Workflow,
		Tag:               input.Tag,
		CommitSHA:         input.CommitSHA,
		RunHeadSHA:        input.RunHeadSHA,
		Version:           input.Version,
	}
	if err := verifyHandoffIdentity(handoff); err != nil {
		return releaseHandoff{}, err
	}

	production, err := describeArtifacts(input.DistDir, releaseArtifactNames)
	if err != nil {
		return releaseHandoff{}, fmt.Errorf("describe production artifacts: %w", err)
	}
	containers, err := describeArtifacts(input.ContainerDir, containerArtifactNames)
	if err != nil {
		return releaseHandoff{}, fmt.Errorf("describe container artifacts: %w", err)
	}
	if len(production) == 0 {
		return releaseHandoff{}, errors.New("handoff requires at least one production artifact")
	}
	if len(containers) == 0 {
		return releaseHandoff{}, errors.New("handoff requires at least one container artifact")
	}
	handoff.ProductionArtifacts = production
	handoff.ContainerArtifacts = containers

	if checksums := filepath.Join(input.DistDir, "checksums.txt"); fileExists(checksums) {
		sum, err := hashFile(checksums)
		if err != nil {
			return releaseHandoff{}, err
		}
		handoff.Checksums = sum
	}

	encoded, err := json.MarshalIndent(handoff, "", "  ")
	if err != nil {
		return releaseHandoff{}, err
	}
	if input.Output != "" {
		if err := os.WriteFile(input.Output, append(encoded, '\n'), 0o600); err != nil {
			return releaseHandoff{}, err
		}
	}
	return handoff, nil
}

// verifyHandoffIdentity 校验 handoff 的身份字段完整性。缺少任一项都意味着该
// handoff 不能被 publisher 当作受信来源（§15）。
func verifyHandoffIdentity(handoff releaseHandoff) error {
	if strings.TrimSpace(handoff.Repository) == "" {
		return errors.New("handoff repository is required")
	}
	if _, _, ok := strings.Cut(handoff.Repository, "/"); !ok {
		return fmt.Errorf("handoff repository must use owner/name form: %q", handoff.Repository)
	}
	if handoff.ReleaseRunID <= 0 {
		return errors.New("handoff release_run_id must be positive")
	}
	if strings.TrimSpace(handoff.Workflow) == "" {
		return errors.New("handoff workflow is required")
	}
	if strings.TrimSpace(handoff.Tag) == "" {
		return errors.New("handoff tag is required")
	}
	if strings.TrimSpace(handoff.CommitSHA) == "" {
		return errors.New("handoff commit_sha is required")
	}
	if strings.TrimSpace(handoff.RunHeadSHA) == "" {
		return errors.New("handoff run_head_sha is required")
	}
	if strings.TrimSpace(handoff.Version) == "" {
		return errors.New("handoff version is required")
	}
	return nil
}

// verifyHandoffArtifacts 使用产物目录内的 checksums.txt 校验。
func verifyHandoffArtifacts(handoff releaseHandoff, distDir, containerDir string) error {
	return verifyHandoffArtifactsAt(handoff, distDir, containerDir, filepath.Join(distDir, "checksums.txt"))
}

// verifyHandoffArtifactsAt 重新计算磁盘上的字节并与 handoff 记录比对。任何缺失、
// 多出或内容不一致都会失败，从而保证 publisher 发布的是同一批 bytes（§19/§31）。
//
// checksumsPath 单独传入，因为 publish 阶段可能尚未在 dist 下生成 checksums.txt
// （它由 preparation 阶段产出）；此时仍应对照那份已批准的校验和。
func verifyHandoffArtifactsAt(handoff releaseHandoff, distDir, containerDir, checksumsPath string) error {
	if err := verifyHandoffIdentity(handoff); err != nil {
		return err
	}
	if len(handoff.ProductionArtifacts) == 0 {
		return errors.New("handoff has no production artifacts")
	}
	if len(handoff.ContainerArtifacts) == 0 {
		return errors.New("handoff has no container artifacts")
	}
	if err := compareArtifacts(handoff.ProductionArtifacts, distDir, releaseArtifactNames); err != nil {
		return fmt.Errorf("production artifacts: %w", err)
	}
	if err := compareArtifacts(handoff.ContainerArtifacts, containerDir, containerArtifactNames); err != nil {
		return fmt.Errorf("container artifacts: %w", err)
	}
	return verifyHandoffChecksums(handoff, checksumsPath)
}

// verifyHandoffIdentityOnly 校验 handoff 的身份，并要求调用方解析出的 run 与
// handoff 记录一致，但不要求本地已下载任何产物。
//
// 有些 publisher 只发布 handoff 里的一小部分内容（例如 ClawHub 只发布 skill），
// 它们仍必须证明「读取的 handoff 来自被我信任的那个 run」，否则等于跳过了 §15
// 的 trusted handoff 校验；同时又不该被迫下载与它无关的归档和镜像。
func verifyHandoffIdentityOnly(handoff releaseHandoff, expectedRunID int64, expectedRepository, expectedTag, expectedRunHeadSHA string) error {
	if err := verifyHandoffIdentity(handoff); err != nil {
		return err
	}
	if expectedRunID != 0 && handoff.ReleaseRunID != expectedRunID {
		return fmt.Errorf("handoff release_run_id = %d, want %d", handoff.ReleaseRunID, expectedRunID)
	}
	if expectedRepository != "" && handoff.Repository != expectedRepository {
		return fmt.Errorf("handoff repository = %q, want %q", handoff.Repository, expectedRepository)
	}
	if expectedTag != "" && handoff.Tag != expectedTag {
		return fmt.Errorf("handoff tag = %q, want %q", handoff.Tag, expectedTag)
	}
	if expectedRunHeadSHA != "" && handoff.RunHeadSHA != expectedRunHeadSHA {
		return fmt.Errorf("handoff run_head_sha = %q, want %q", handoff.RunHeadSHA, expectedRunHeadSHA)
	}
	return nil
}

// verifyHandoffSectionAt 只校验 handoff 中某一个 section 的产物。downstream
// publisher 往往只下载它需要发布的那类产物（例如 Homebrew 只拿 release 归档、
// 容器 publisher 只拿镜像 tar），因此必须能按 section 校验而不强制下载全部。
func verifyHandoffSectionAt(handoff releaseHandoff, section, dir, checksumsPath string) error {
	if err := verifyHandoffIdentity(handoff); err != nil {
		return err
	}
	switch section {
	case handoffSectionProduction:
		if len(handoff.ProductionArtifacts) == 0 {
			return errors.New("handoff has no production artifacts")
		}
		if err := compareArtifacts(handoff.ProductionArtifacts, dir, releaseArtifactNames); err != nil {
			return fmt.Errorf("production artifacts: %w", err)
		}
	case handoffSectionContainer:
		if len(handoff.ContainerArtifacts) == 0 {
			return errors.New("handoff has no container artifacts")
		}
		if err := compareArtifacts(handoff.ContainerArtifacts, dir, containerArtifactNames); err != nil {
			return fmt.Errorf("container artifacts: %w", err)
		}
	default:
		return fmt.Errorf("unknown handoff section %q", section)
	}
	return verifyHandoffChecksums(handoff, checksumsPath)
}

const (
	handoffSectionProduction = "production"
	handoffSectionContainer  = "container"
)

func verifyHandoffChecksums(handoff releaseHandoff, checksumsPath string) error {
	if handoff.Checksums != "" {
		sum, err := hashFile(checksumsPath)
		if err != nil {
			return fmt.Errorf("checksums %s: %w", checksumsPath, err)
		}
		if sum != handoff.Checksums {
			return fmt.Errorf("checksums %s = %s, want %s", checksumsPath, sum, handoff.Checksums)
		}
	}
	return nil
}

// compareArtifacts 逐项校验记录与磁盘字节，并要求“选择器看见的集合”与记录完全一致。
// 选择器而不是整个目录，是因为 dist 还包含 checksums/manifest 等派生文件。
func compareArtifacts(recorded []handoffArtifact, dir string, selectNames func(string) ([]string, error)) error {
	for _, artifact := range recorded {
		if !isSimpleFileName(artifact.Name) {
			return fmt.Errorf("artifact name is not a simple file name: %q", artifact.Name)
		}
		path := filepath.Join(dir, artifact.Name)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("artifact %s: %w", artifact.Name, err)
		}
		if info.IsDir() {
			return fmt.Errorf("artifact %s is a directory", artifact.Name)
		}
		if artifact.Size > 0 && info.Size() != artifact.Size {
			return fmt.Errorf("artifact %s size = %d, want %d", artifact.Name, info.Size(), artifact.Size)
		}
		sum, err := hashFile(path)
		if err != nil {
			return err
		}
		if sum != artifact.SHA256 {
			return fmt.Errorf("artifact %s sha256 = %s, want %s", artifact.Name, sum, artifact.SHA256)
		}
	}
	actual, err := selectNames(dir)
	if err != nil {
		return err
	}
	expected := make([]string, 0, len(recorded))
	for _, artifact := range recorded {
		expected = append(expected, artifact.Name)
	}
	sort.Strings(expected)
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		return fmt.Errorf("artifact set mismatch\nexpected:\n%s\nactual:\n%s",
			strings.Join(expected, "\n"), strings.Join(actual, "\n"))
	}
	return nil
}

// isSimpleFileName 拒绝路径分隔符与 parent 引用，避免 handoff 指定目录外的文件。
func isSimpleFileName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	return filepath.Base(name) == name
}

func describeArtifacts(dir string, selectNames func(string) ([]string, error)) ([]handoffArtifact, error) {
	names, err := selectNames(dir)
	if err != nil {
		return nil, err
	}
	artifacts := make([]handoffArtifact, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		sum, err := hashFile(path)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, handoffArtifact{Name: name, SHA256: sum, Size: info.Size()})
	}
	return artifacts, nil
}

// releaseArtifactNames 返回目录中的 release 归档（按名字排序）。校验和与 handoff
// 只覆盖归档本身，不覆盖派生文件。
func releaseArtifactNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func containerArtifactNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(name, "pixiv-cli-") && strings.HasSuffix(name, ".tar") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// writeHandoffCommand 在 Release preparation 阶段固化不可变交接清单。
func writeHandoffCommand(args []string) {
	set := flag.NewFlagSet("write-handoff", flag.ExitOnError)
	repository := set.String("repository", "", "GitHub repository in owner/name form")
	runID := set.Int64("run-id", 0, "Release workflow run ID")
	runAttempt := set.Int("run-attempt", 0, "optional Release workflow run attempt")
	workflow := set.String("workflow", "Release", "expected Release workflow name")
	tag := set.String("tag", "", "immutable release tag")
	commitSHA := set.String("commit-sha", "", "immutable release tag commit SHA")
	runHeadSHA := set.String("run-head-sha", "", "release workflow run head SHA")
	version := set.String("version", "", "release version without v")
	distDir := set.String("dist-dir", "dist", "production artifact directory")
	containerDir := set.String("container-dir", "containers", "container artifact directory")
	output := set.String("output", "", "handoff output path")
	_ = set.Parse(args)
	if *output == "" {
		fatal(errors.New("output is required"))
	}
	if _, err := writeReleaseHandoff(releaseHandoffInput{
		Repository:   *repository,
		RunID:        *runID,
		RunAttempt:   *runAttempt,
		Workflow:     *workflow,
		Tag:          *tag,
		CommitSHA:    *commitSHA,
		RunHeadSHA:   *runHeadSHA,
		Version:      *version,
		DistDir:      *distDir,
		ContainerDir: *containerDir,
		Output:       *output,
	}); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stdout, "handoff=%s\n", *output)
}

// verifyHandoffSetCommand 供 downstream publisher 验正它拿到的产物确实属于
// handoff 记录的那批 bytes。提供的身份参数会与 handoff 内记录的值比对，避免
// 发布错 run 的产物（§15/§18）。
func verifyHandoffSetCommand(args []string) {
	set := flag.NewFlagSet("verify-handoff-set", flag.ExitOnError)
	handoffPath := set.String("handoff", "", "release handoff JSON path")
	distDir := set.String("dist-dir", "dist", "production artifact directory")
	containerDir := set.String("container-dir", "containers", "container artifact directory")
	repository := set.String("repository", "", "optional expected repository")
	runID := set.Int64("run-id", 0, "optional expected Release workflow run ID")
	tag := set.String("tag", "", "optional expected immutable release tag")
	headSHA := set.String("run-head-sha", "", "optional expected release run head SHA")
	checksumsPath := set.String("checksums", "", "optional checksums.txt path (default: <dist-dir>/checksums.txt)")
	section := set.String("section", "", "verify only one section: production or container")
	_ = set.Parse(args)
	if *handoffPath == "" {
		fatal(errors.New("handoff is required"))
	}
	resolvedChecksums := *checksumsPath
	if resolvedChecksums == "" {
		resolvedChecksums = filepath.Join(*distDir, "checksums.txt")
	}
	body, err := os.ReadFile(*handoffPath)
	if err != nil {
		fatal(err)
	}
	var handoff releaseHandoff
	if err := json.Unmarshal(body, &handoff); err != nil {
		fatal(fmt.Errorf("decode handoff: %w", err))
	}
	if *repository != "" && handoff.Repository != *repository {
		fatal(fmt.Errorf("handoff repository = %q, want %q", handoff.Repository, *repository))
	}
	if *runID != 0 && handoff.ReleaseRunID != *runID {
		fatal(fmt.Errorf("handoff release_run_id = %d, want %d", handoff.ReleaseRunID, *runID))
	}
	if *tag != "" && handoff.Tag != *tag {
		fatal(fmt.Errorf("handoff tag = %q, want %q", handoff.Tag, *tag))
	}
	// §15：publisher 必须确认自己读取的 handoff 确实由所解析出的那个 run 产生。
	// 这里比对 handoff 记录的 run head SHA 与调用方从该 run 读到的 head SHA；
	// 它是自校验的，不会把 tag commit 与 dispatch 分支头混为一谈。
	if *headSHA != "" && handoff.RunHeadSHA != *headSHA {
		fatal(fmt.Errorf("handoff run_head_sha = %q, want %q", handoff.RunHeadSHA, *headSHA))
	}
	if *section != "" {
		dir := *distDir
		if *section == handoffSectionContainer {
			dir = *containerDir
		}
		if err := verifyHandoffSectionAt(handoff, *section, dir, resolvedChecksums); err != nil {
			fatal(err)
		}
	} else if err := verifyHandoffArtifactsAt(handoff, *distDir, *containerDir, resolvedChecksums); err != nil {
		fatal(err)
	}
	outputPath := strings.TrimSpace(os.Getenv("GITHUB_OUTPUT"))
	if outputPath != "" {
		if err := writeHandoffOutputs(outputPath, handoff); err != nil {
			fatal(err)
		}
	}
	fmt.Fprintf(os.Stdout, "handoff verified: %s run %d (%d production, %d container)\n",
		handoff.Tag, handoff.ReleaseRunID, len(handoff.ProductionArtifacts), len(handoff.ContainerArtifacts))
}

// writeHandoffOutputs 把 publisher 需要的 handoff 身份字段发布为 GitHub Actions
// output，使 workflow 不再从 tag 名或 run 元数据自行推断（§17）。
func writeHandoffOutputs(path string, handoff releaseHandoff) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	values := []struct{ key, value string }{
		{"repository", handoff.Repository},
		{"release_run_id", strconv.FormatInt(handoff.ReleaseRunID, 10)},
		{"tag", handoff.Tag},
		{"commit_sha", handoff.CommitSHA},
		{"version", handoff.Version},
	}
	for _, entry := range values {
		if strings.ContainsAny(entry.value, "\r\n") {
			return fmt.Errorf("GitHub output %s contains a newline", entry.key)
		}
		if _, err := fmt.Fprintf(file, "%s=%s\n", entry.key, entry.value); err != nil {
			return err
		}
	}
	return nil
}

// verifyHandoffIdentityCommand 供只发布 handoff 中部分内容的 publisher 使用：
// 它证明 handoff 来自被解析的那个 run，而不要求本地已下载任何产物（§15）。
func verifyHandoffIdentityCommand(args []string) {
	set := flag.NewFlagSet("verify-handoff-identity", flag.ExitOnError)
	handoffPath := set.String("handoff", "", "release handoff JSON path")
	runID := set.Int64("run-id", 0, "expected Release workflow run ID")
	repository := set.String("repository", "", "expected repository in owner/name form")
	tag := set.String("tag", "", "expected immutable release tag")
	runHeadSHA := set.String("run-head-sha", "", "expected release run head SHA")
	_ = set.Parse(args)
	if *handoffPath == "" {
		fatal(errors.New("handoff is required"))
	}
	body, err := os.ReadFile(*handoffPath)
	if err != nil {
		fatal(err)
	}
	var handoff releaseHandoff
	if err := json.Unmarshal(body, &handoff); err != nil {
		fatal(fmt.Errorf("decode handoff: %w", err))
	}
	if err := verifyHandoffIdentityOnly(handoff, *runID, *repository, *tag, *runHeadSHA); err != nil {
		fatal(err)
	}
	outputPath := strings.TrimSpace(os.Getenv("GITHUB_OUTPUT"))
	if outputPath != "" {
		if err := writeHandoffOutputs(outputPath, handoff); err != nil {
			fatal(err)
		}
	}
	fmt.Fprintf(os.Stdout, "handoff identity verified: %s run %d\n", handoff.Tag, handoff.ReleaseRunID)
}
