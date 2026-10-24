// 许可证 bundle 的依赖枚举：按发布目标执行离线 go list，并解析模块元数据。

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type goModule struct {
	Path, Version, Dir string
	Main               bool
	Replace            *goModule
}

type attributions struct {
	Ports         []packageMetadata `json:"ports"`
	ModuleNotices []packageMetadata `json:"module_notices"`
}

func collectDependencies(goCommand, repository string) ([]bundledPackage, error) {
	modules := map[string]packageMetadata{}
	for _, target := range releaseTargets {
		body, err := goMetadataJSON(goCommand, repository, target)
		if err != nil {
			return nil, err
		}
		packages, err := modulesFromMetadata(body)
		if err != nil {
			return nil, fmt.Errorf("decode Go metadata for %s: %w", target, err)
		}
		for _, pkg := range packages {
			key := pkg.Name + "@" + pkg.Version
			if previous, exists := modules[key]; exists && previous != pkg {
				return nil, fmt.Errorf("module %s resolves to conflicting sources", key)
			}
			modules[key] = pkg
		}
	}
	notices, err := loadAttributions(repository)
	if err != nil {
		return nil, err
	}
	for _, notice := range notices.ModuleNotices {
		key := notice.Name + "@" + notice.Version
		if _, used := modules[key]; used {
			// 仅使用已审计、版本精确匹配的源声明，不为任意缺许可证模块降级。
			modules[key] = notice
		}
	}
	for _, port := range notices.Ports {
		key := port.Name + "@" + port.Version
		if _, exists := modules[key]; exists {
			return nil, fmt.Errorf("duplicate attribution %s", key)
		}
		modules[key] = port
	}
	bundle := make([]bundledPackage, 0, len(modules))
	directories := map[string]bool{}
	for _, pkg := range modules {
		dir := strings.ToLower(packageDirectory(pkg))
		if directories[dir] {
			return nil, fmt.Errorf("conflicting license output directory %s", dir)
		}
		directories[dir] = true
		files, err := packageLicenseFiles(pkg)
		if err != nil {
			return nil, err
		}
		bundle = append(bundle, bundledPackage{Package: pkg, Files: files})
	}
	sort.Slice(bundle, func(i, j int) bool {
		if bundle[i].Package.Name == bundle[j].Package.Name {
			return bundle[i].Package.Version < bundle[j].Package.Version
		}
		return bundle[i].Package.Name < bundle[j].Package.Name
	})
	return bundle, nil
}

func goMetadataJSON(goCommand, repository string, target releaseTarget) ([]byte, error) {
	command := exec.Command(goCommand, "list", "-mod=readonly", "-deps", "-json", "./cmd/pixiv")
	command.Dir = repository
	// 与无 cgo 的默认发布图一致；不让本机 workspace、GOFLAGS 或自动下载改变闭包。
	command.Env = append(os.Environ(), "GOOS="+target.GOOS, "GOARCH="+target.GOARCH,
		"CGO_ENABLED=0", "GOENV=off", "GOPRIVATE=", "GONOPROXY=none", "GOWORK=off", "GOFLAGS=", "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("go list production dependencies for %s: %w: %s", target, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func modulesFromMetadata(body []byte) ([]packageMetadata, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var modules []packageMetadata
	for {
		var pkg struct{ Module *goModule }
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			return modules, nil
		} else if err != nil {
			return nil, err
		}
		m := pkg.Module
		if m == nil || m.Main {
			continue
		}
		if m.Path == "" || m.Version == "" || m.Dir == "" {
			return nil, errors.New("dependency module is missing path, version or source directory")
		}
		notice := packageMetadata{Name: m.Path, Version: m.Version, Directory: m.Dir, Note: "Production Go module"}
		if m.Replace != nil {
			notice.Note += "; replaced by " + m.Replace.Path + " " + m.Replace.Version
		}
		modules = append(modules, notice)
	}
}

func loadAttributions(repository string) (attributions, error) {
	var notices attributions
	body, err := os.ReadFile(filepath.Join(repository, "third_party", "attributions.json"))
	if err != nil {
		return notices, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&notices); err != nil {
		return notices, fmt.Errorf("decode source attributions: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return notices, errors.New("source attributions have trailing JSON")
	}
	seen := map[string]bool{}
	for _, group := range [][]packageMetadata{notices.Ports, notices.ModuleNotices} {
		for i := range group {
			p := &group[i]
			if p.Name == "" || p.Version == "" || !filepath.IsLocal(p.Directory) || p.Directory == "." {
				return notices, errors.New("source attribution requires a name, version and repository-relative directory")
			}
			key := p.Name + "@" + p.Version
			if seen[key] {
				return notices, fmt.Errorf("duplicate attribution %s", key)
			}
			seen[key] = true
			p.Directory = filepath.Join(repository, filepath.FromSlash(p.Directory))
		}
	}
	return notices, nil
}
