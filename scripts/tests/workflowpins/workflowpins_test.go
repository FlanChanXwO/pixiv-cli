// Package workflowpins_test 锁住全仓 GitHub Actions 的 Node runtime 与 pin 形式。
// GitHub 已弃用 Node 20 runner；旧 action 会被强制迁移，产生 deprecation warning
// 并在未来失败。这里同时锁定 full SHA pin，禁止回退到可移动的 tag。
package workflowpins_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// node24Actions 是允许出现在 workflow 中的 action 及其唯一 commit SHA。
// 前四项都是发布时已确认 using: node24 的稳定版本：
//
//	actions/checkout          v7.0.1
//	actions/setup-go          v7.0.0
//	actions/download-artifact v8.0.1
//	actions/upload-artifact   v7.0.1
//
// 其余 pin 不属于本次升级范围，但仍必须保持 full SHA 形式。
var node24Actions = map[string]string{
	"actions/checkout":          "3d3c42e5aac5ba805825da76410c181273ba90b1",
	"actions/setup-go":          "b7ad1dad31e06c5925ef5d2fc7ad053ef454303e",
	"actions/download-artifact": "3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c",
	"actions/upload-artifact":   "043fb46d1a93c77aae656e7c1c64a875d1fc6a0a",
}

// retiredPins 是升级前的 Node 20 pin；任何残留都意味着 runtime 仍在被弃用的
// Node 上运行。
var retiredPins = map[string]string{
	"actions/checkout":          "34e114876b0b11c390a56381ad16ebd13914f8d5",
	"actions/setup-go":          "40f1582b2485089dde7abd97c1529aa768e1baff",
	"actions/download-artifact": "d3f86a106a0bac45b974a628896c90dbdf5c8093",
	"actions/upload-artifact":   "ea165f8d65b6e75b540449e92b4886f43607fa02",
}

// usesPattern 只匹配作为独立 YAML key 出现的 uses:，避免命中 `statuses: write`
// 这类把 "uses:" 当作子串包含的值。
var usesPattern = regexp.MustCompile(`(?:^|[ \t])uses:[ \t]*([^ \t\n#]+)`)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

func workflowFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repositoryRoot(t), ".github", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yml") || strings.HasSuffix(entry.Name(), ".yaml")) {
			paths = append(paths, filepath.Join(repositoryRoot(t), ".github", "workflows", entry.Name()))
		}
	}
	if len(paths) == 0 {
		t.Fatal("no workflow files found")
	}
	return paths
}

// TestWorkflowsUseNode24ActionPins 覆盖 R12：这四类 action 必须 pin 到已确认
// using: node24 的稳定版本，且不得残留任何 Node 20 pin。
func TestWorkflowsUseNode24ActionPins(t *testing.T) {
	t.Parallel()

	seen := make(map[string]int)
	for _, path := range workflowFiles(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		for _, match := range usesPattern.FindAllStringSubmatch(string(body), -1) {
			reference := match[1]
			action, sha, ok := strings.Cut(reference, "@")
			if !ok {
				continue
			}
			if wants, tracked := node24Actions[action]; tracked {
				seen[action]++
				if sha != wants {
					t.Errorf("%s: %s is pinned to %s, want node24 %s", name, action, sha, wants)
				}
			}
			if retired, tracked := retiredPins[action]; tracked && sha == retired {
				t.Errorf("%s: %s still uses the retired Node 20 pin %s", name, action, retired)
			}
		}
	}
	for action := range node24Actions {
		if seen[action] == 0 {
			t.Errorf("%s is no longer referenced by any workflow", action)
		}
	}
}

// TestWorkflowActionsUseFullSHAPins 锁定 pin 形式：full commit SHA 是不可变的，
// 而 tag（如 @v7）可以被上游重新指向。任何非 SHA 形式都是供应链回归。
func TestWorkflowActionsUseFullSHAPins(t *testing.T) {
	t.Parallel()

	fullSHA := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, path := range workflowFiles(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		for _, match := range usesPattern.FindAllStringSubmatch(string(body), -1) {
			reference := match[1]
			// 本地 action（./path）与 docker action 不适用 SHA pin。
			if strings.HasPrefix(reference, "./") || strings.HasPrefix(reference, "docker://") {
				continue
			}
			action, sha, ok := strings.Cut(reference, "@")
			if !ok {
				t.Errorf("%s: action %q has no pin", name, reference)
				continue
			}
			if !fullSHA.MatchString(sha) {
				t.Errorf("%s: action %s is pinned to %q, want a full commit SHA", name, action, sha)
			}
		}
	}
}
