package nativeevidence

import (
	"regexp"
)

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

var gitCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

const evidenceSchemaVersion = 2

type nativeTarget struct {
	goos       string
	goarch     string
	rustTarget string
	staticlib  string
}

var nativeTargets = map[string]nativeTarget{
	"darwin/amd64":  {goos: "darwin", goarch: "amd64", rustTarget: "x86_64-apple-darwin", staticlib: "libugoira_rs.a"},
	"darwin/arm64":  {goos: "darwin", goarch: "arm64", rustTarget: "aarch64-apple-darwin", staticlib: "libugoira_rs.a"},
	"linux/amd64":   {goos: "linux", goarch: "amd64", rustTarget: "x86_64-unknown-linux-gnu", staticlib: "libugoira_rs.a"},
	"linux/arm64":   {goos: "linux", goarch: "arm64", rustTarget: "aarch64-unknown-linux-gnu", staticlib: "libugoira_rs.a"},
	"windows/amd64": {goos: "windows", goarch: "amd64", rustTarget: "x86_64-pc-windows-msvc", staticlib: "ugoira_rs.lib"},
	"windows/arm64": {goos: "windows", goarch: "arm64", rustTarget: "aarch64-pc-windows-msvc", staticlib: "ugoira_rs.lib"},
}

type recordOptions struct {
	repoRoot     string
	version      string
	sourceCommit string
	target       string
	rustTarget   string
	staticlib    string
	binary       string
	archive      string
	output       string
}

// evidenceRecord 是单个真实 runner 输出的可迁移审计记录。测试可构造 fixture 验证格式，
// 但只有 workflow 产生、绑定 main SHA 的记录才是 Task 33 所要求的 native evidence。
type evidenceRecord struct {
	Schema int            `json:"schema"`
	Target evidenceTarget `json:"target"`
	// SourceCommit 由 workflow 上下文提供，不再从 binary runtime 元数据推断。
	SourceCommit string          `json:"source_commit"`
	SourceDigest string          `json:"source_digest"`
	Staticlib    evidenceFile    `json:"staticlib"`
	Binary       evidenceBinary  `json:"binary"`
	Archive      evidenceArchive `json:"archive"`
}

type evidenceTarget struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	RustTarget string `json:"rust_target"`
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
