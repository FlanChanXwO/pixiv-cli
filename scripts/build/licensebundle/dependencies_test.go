package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunUnionsSixTargetGoProductionModules(t *testing.T) {
	dir := t.TempDir()
	common := makeModuleFixture(t, dir, "example.com/common", "v1.0.0")
	windows := makeModuleFixture(t, dir, "example.com/windows", "v2.0.0")
	metadata := map[string]string{}
	for _, target := range []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
		metadata[target] = moduleJSON(t, map[string]any{"Path": "example.com/main", "Main": true, "Dir": dir}) + moduleJSON(t, common)
		if strings.HasPrefix(target, "windows/") {
			metadata[target] += moduleJSON(t, windows)
		}
	}
	writeAttributionFixture(t, dir, `{"ports":[],"module_notices":[]}`)
	record := filepath.Join(dir, "queries.txt")
	goCommand := buildFakeGo(t, dir, record, metadata)
	args := []string{"--repository", dir, "--go", goCommand}
	if err := run(args); err != nil {
		t.Fatalf("generate production Go notices: %v", err)
	}
	for _, name := range []string{"example.com_common-v1.0.0", "example.com_windows-v2.0.0"} {
		body, err := os.ReadFile(filepath.Join(dir, "third_party", "licenses", name, "LICENSE"))
		if err != nil || string(body) != "fixture license body\n" {
			t.Fatalf("missing module notice %s: %q %v", name, body, err)
		}
	}
	body, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(body)); strings.Join(got, ",") != "darwin/amd64,darwin/arm64,linux/amd64,linux/arm64,windows/amd64,windows/arm64" {
		t.Fatalf("Go targets = %v", got)
	}
	if err := run(append(args, "--check")); err != nil {
		t.Fatalf("fresh bundle check: %v", err)
	}
	index := filepath.Join(dir, "THIRD_PARTY_LICENSES.md")
	originalIndex, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, []byte("stale index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(append(args, "--check")); err == nil || !strings.Contains(err.Error(), "index is stale") {
		t.Fatalf("stale index was accepted: %v", err)
	}
	if err := os.WriteFile(index, originalIndex, 0o600); err != nil {
		t.Fatal(err)
	}
	license := filepath.Join(dir, "third_party/licenses/example.com_common-v1.0.0/LICENSE")
	if err := os.WriteFile(license, []byte("tampered text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(append(args, "--check")); err == nil || !strings.Contains(err.Error(), "license directory is stale") {
		t.Fatalf("tampered license text was accepted: %v", err)
	}
	if body, err := os.ReadFile(license); err != nil || string(body) != "tampered text" {
		t.Fatalf("--check published changes: %q %v", body, err)
	}
}

func makeModuleFixture(t *testing.T, root, path, version string) map[string]any {
	t.Helper()
	dir := filepath.Join(root, strings.ReplaceAll(path, "/", "_"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("fixture license body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return map[string]any{"Path": path, "Version": version, "Dir": dir}
}

func moduleJSON(t *testing.T, module map[string]any) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"Module": module})
	if err != nil {
		t.Fatal(err)
	}
	return string(body) + "\n"
}

func writeAttributionFixture(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "third_party"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "third_party", "attributions.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// 使用真实的跨平台临时可执行文件，检查生产 metadata 命令与离线环境。
func buildFakeGo(t *testing.T, dir, record string, metadata map[string]string) string {
	t.Helper()
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`package main
import("encoding/json";"fmt";"os";"strings")
func main(){
 if strings.Join(os.Args[1:], " ") != "list -mod=readonly -deps -json ./cmd/pixiv" { os.Exit(2) }
 for k,v := range map[string]string{"CGO_ENABLED":"0","GOENV":"off","GONOPROXY":"none","GOPROXY":"off","GOSUMDB":"off","GOTOOLCHAIN":"local","GOWORK":"off","GOFLAGS":""} { if os.Getenv(k)!=v { os.Exit(3) } }
 var metadata map[string]string
 if err:=json.Unmarshal([]byte(%q), &metadata);err!=nil{panic(err)}
 target:=os.Getenv("GOOS")+"/"+os.Getenv("GOARCH")
 body,ok:=metadata[target];if !ok { os.Exit(4) }
 f,err:=os.OpenFile(%q,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err!=nil{panic(err)}
 if _,err:=fmt.Fprintln(f,target);err!=nil{panic(err)};if err:=f.Close();err!=nil{panic(err)}
 fmt.Print(body)
}`, string(data), record)
	path := filepath.Join(dir, "fake-go.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "fake-go")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if output, err := exec.Command("go", "build", "-o", binary, path).CombinedOutput(); err != nil {
		t.Fatalf("build Go metadata fixture: %v\n%s", err, output)
	}
	return binary
}

func TestRunMissingNoticesPreservesPublishedBundle(t *testing.T) {
	dir := t.TempDir()
	module := makeModuleFixture(t, dir, "example.com/dependency", "v1.0.0")
	port := makeModuleFixture(t, dir, "ported", "1.0.0")
	catalog := `{"ports":[{"name":"ported","version":"1.0.0","directory":"ported"}],"module_notices":[]}`
	writeAttributionFixture(t, dir, catalog)
	metadata := map[string]string{}
	for _, target := range releaseTargets {
		metadata[target.String()] = moduleJSON(t, module)
	}
	command := buildFakeGo(t, dir, filepath.Join(dir, "queries"), metadata)
	args := []string{"--repository", dir, "--go", command}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, "THIRD_PARTY_LICENSES.md")
	original, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing module license", "empty module license", "missing port license", "wrong notice version", "unsafe source path", "malformed catalog"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(module["Dir"].(string), "LICENSE")
			switch name {
			case "missing module license", "wrong notice version":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "empty module license":
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing port license":
				path = filepath.Join(port["Dir"].(string), "LICENSE")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				if err := os.WriteFile(path, []byte("fixture license body\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				writeAttributionFixture(t, dir, catalog)
			})
			if name == "wrong notice version" {
				writeAttributionFixture(t, dir, `{"ports":[],"module_notices":[{"name":"example.com/dependency","version":"v2.0.0","directory":"ported"}]}`)
			}
			if name == "unsafe source path" {
				writeAttributionFixture(t, dir, `{"ports":[{"name":"bad","version":"1.0.0","directory":"../outside"}]}`)
			}
			if name == "malformed catalog" {
				writeAttributionFixture(t, dir, `{"unknown":true}`)
			}
			if err := run(args); err == nil {
				t.Fatal("invalid source notices were accepted")
			}
			body, err := os.ReadFile(index)
			if err != nil || string(body) != string(original) {
				t.Fatalf("published index changed on failure: %v", err)
			}
			body, err = os.ReadFile(filepath.Join(dir, "third_party/licenses/example.com_dependency-v1.0.0/LICENSE"))
			if err != nil || string(body) != "fixture license body\n" {
				t.Fatalf("published license tree changed on failure: %v", err)
			}
		})
	}
}
