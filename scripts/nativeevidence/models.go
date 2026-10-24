// schema 3 证据与发布目标的共享类型。

package main

import (
	"regexp"
)

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

var gitCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

const evidenceSchemaVersion = 3

type nativeTarget struct {
	goos   string
	goarch string
}

var nativeTargets = map[string]nativeTarget{
	"darwin/amd64":  {goos: "darwin", goarch: "amd64"},
	"darwin/arm64":  {goos: "darwin", goarch: "arm64"},
	"linux/amd64":   {goos: "linux", goarch: "amd64"},
	"linux/arm64":   {goos: "linux", goarch: "arm64"},
	"windows/amd64": {goos: "windows", goarch: "amd64"},
	"windows/arm64": {goos: "windows", goarch: "arm64"},
}

type recordOptions struct {
	repoRoot     string
	version      string
	sourceCommit string
	target       string
	binary       string
	archive      string
	output       string
}

// evidenceRecord 保存单个 runner 的 binary/archive 声明；源码身份由受审计 workflow 提供。
type evidenceRecord struct {
	Schema int            `json:"schema"`
	Target evidenceTarget `json:"target"`
	// SourceCommit 由 workflow 上下文提供，不再从 binary runtime 元数据推断。
	SourceCommit string          `json:"source_commit"`
	Binary       evidenceBinary  `json:"binary"`
	Archive      evidenceArchive `json:"archive"`
}

type evidenceTarget struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

type evidenceFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type evidenceBinary struct {
	evidenceFile
	Version string `json:"version"`
}

type evidenceArchive struct {
	evidenceFile
	Members []evidenceFile `json:"members"`
}

type consolidateOptions struct {
	repoRoot        string
	expectedVersion string
	expectedCommit  string
	inputDir        string
	outputDir       string
}

type evidenceLocation struct {
	record evidenceRecord
	dir    string
}

// evidenceSummary 的每条记录对应同名 GOOS-GOARCH 子目录中的完整 artifact。
type evidenceSummary struct {
	Schema       int              `json:"schema"`
	SourceCommit string           `json:"source_commit"`
	Version      string           `json:"version"`
	Records      []evidenceRecord `json:"records"`
}
