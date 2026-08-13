package architecture_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const modulePath = "github.com/FlanChanXwO/pixiv-cli"

// packageGraph 是 go list 给出的当前编译图。它只把 Go toolchain 作为已安装的
// 项目基础工具使用，不依赖网络、编辑器扩展或本机绝对路径。
type packageGraph struct {
	Packages map[string]goListPackage
}

type goListPackage struct {
	Dir             string
	ImportPath      string
	Name            string
	GoFiles         []string
	CgoFiles        []string
	CompiledGoFiles []string
	Imports         []string
	Deps            []string
	Error           *struct {
		Err string
	}
}

type sourceFile struct {
	Path string
	Name string
	AST  *ast.File
	FSet *token.FileSet
}

type sourceRepository struct {
	Root  string
	Graph packageGraph
	Files []sourceFile
}

var repositoryCache struct {
	sync.Once
	repository *sourceRepository
	err        error
}

func architectureRepository(t *testing.T) *sourceRepository {
	t.Helper()
	repositoryCache.Do(func() {
		root := repositoryRoot()
		repositoryCache.repository, repositoryCache.err = loadSourceRepository(root)
	})
	if repositoryCache.err != nil {
		t.Fatal(repositoryCache.err)
	}
	return repositoryCache.repository
}

func loadSourceRepository(root string) (*sourceRepository, error) {
	graph, err := loadPackageGraph(root)
	if err != nil {
		return nil, err
	}
	files := make([]sourceFile, 0)
	for _, relativeRoot := range []string{"cmd", "internal", "sdk", "scripts"} {
		base := filepath.Join(root, relativeRoot)
		if _, err := os.Stat(base); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat architecture source root %s: %w", relativeRoot, err)
		}
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				return nil
			}
			fileSet := token.NewFileSet()
			parsed, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse %s: %w", filepath.ToSlash(relativePath(root, path)), err)
			}
			files = append(files, sourceFile{Path: relativePath(root, path), Name: parsed.Name.Name, AST: parsed, FSet: fileSet})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return &sourceRepository{Root: root, Graph: graph, Files: files}, nil
}

func loadPackageGraph(root string) (packageGraph, error) {
	command := exec.Command("go", "list", "-json", "-e", "./cmd/...", "./internal/...", "./sdk/...", "./scripts/...")
	command.Dir = root
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		// 不把 go list 可能包含的机器路径复制进测试错误；命令错误仍保留给
		// 调试者，具体编译/测试失败由项目既有门禁负责展示。
		return packageGraph{}, fmt.Errorf("load Go package graph with go list: %w", err)
	}
	graph := packageGraph{Packages: make(map[string]goListPackage)}
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var packageInfo goListPackage
		err := decoder.Decode(&packageInfo)
		if err == io.EOF {
			break
		}
		if err != nil {
			return packageGraph{}, fmt.Errorf("decode go list package graph: %w", err)
		}
		if packageInfo.ImportPath == "" {
			return packageGraph{}, fmt.Errorf("go list returned a package without ImportPath")
		}
		if packageInfo.Error != nil {
			return packageGraph{}, fmt.Errorf("go list package %s: %s", packageInfo.ImportPath, packageInfo.Error.Err)
		}
		graph.Packages[packageInfo.ImportPath] = packageInfo
	}
	if len(graph.Packages) == 0 {
		return packageGraph{}, fmt.Errorf("go list returned an empty package graph")
	}
	return graph, nil
}

func repositoryRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve architecture test location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func packagePath(relative string) string {
	directory := filepath.ToSlash(filepath.Dir(relative))
	if directory == "." || directory == "" {
		return modulePath
	}
	return modulePath + "/" + directory
}

func isTestFile(path string) bool {
	return strings.HasSuffix(filepath.Base(path), "_test.go")
}

func (r *sourceRepository) filesInPackage(relativeDirectory string, tests bool) []sourceFile {
	result := make([]sourceFile, 0)
	for _, file := range r.Files {
		if filepath.ToSlash(filepath.Dir(file.Path)) != relativeDirectory || isTestFile(file.Path) != tests {
			continue
		}
		result = append(result, file)
	}
	return result
}

func (r *sourceRepository) productionImports() map[string]map[string]struct{} {
	imports := make(map[string]map[string]struct{})
	for _, file := range r.Files {
		if isTestFile(file.Path) {
			continue
		}
		pkgPath := packagePath(file.Path)
		if imports[pkgPath] == nil {
			imports[pkgPath] = make(map[string]struct{})
		}
		for _, spec := range file.AST.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			imports[pkgPath][value] = struct{}{}
		}
	}
	return imports
}

func isInternalImport(imported string) bool {
	return imported == modulePath+"/internal" || strings.HasPrefix(imported, modulePath+"/internal/")
}

func hasPrefixPath(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// retiredDirectoryAllowlist 只记录当前工作树仍存在的旧 owner。它不是永久兼容
// 清单：对应迁移 task 删除目录后，必须在同一个 task 中删除该项；不存在的旧目录
// 不在这里登记，以免无意中把新建目录变成永久例外。
var retiredDirectoryAllowlist = map[string]string{}

// retiredPackageAllowlist 以包为粒度阻止退休目录下出现新的 Go package。当前
// 迁移仍需在旧 owner 内编译；因此允许的是已知 package，而不是整个目录前缀。
var retiredPackageAllowlist = map[string]string{}

var retiredFileAllowlist = map[string]string{}

func TestRetiredPathsAreExplicitlyAllowlisted(t *testing.T) {
	repository := architectureRepository(t)
	for path, task := range retiredDirectoryAllowlist {
		if task == "" {
			t.Errorf("retired directory %s has no deletion task", path)
		}
		info, err := os.Stat(filepath.Join(repository.Root, filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("current retired directory %s must be checked until %s: %v", path, task, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("retired path %s is not a directory", path)
		}
	}
	for path, task := range retiredFileAllowlist {
		if task == "" {
			t.Errorf("retired file %s has no deletion task", path)
		}
		info, err := os.Stat(filepath.Join(repository.Root, filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("current retired file %s must be checked until %s: %v", path, task, err)
			continue
		}
		if !info.Mode().IsRegular() {
			t.Errorf("retired path %s is not a regular file", path)
		}
	}

	for importPath := range repository.Graph.Packages {
		if retiredPath, ok := retiredPackageRoot(importPath); ok && !retiredPackageAllowed(importPath) {
			t.Errorf("new Go package under retired path %s: %s", retiredPath, importPath)
		}
	}
}

func retiredPackageRoots() map[string]struct{} {
	result := make(map[string]struct{})
	for path := range retiredDirectoryAllowlist {
		result[modulePath+"/"+path] = struct{}{}
	}
	return result
}

func retiredPackageRoot(importPath string) (string, bool) {
	roots := make([]string, 0, len(retiredPackageRoots()))
	for root := range retiredPackageRoots() {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		if hasPrefixPath(importPath, root) {
			return root, true
		}
	}
	return "", false
}

func retiredPackageAllowed(importPath string) bool {
	if _, ok := retiredPackageRoot(importPath); !ok {
		return true
	}
	return retiredPackageAllowlist[importPath] != ""
}

var genericProductionFileAllowlist = map[string]string{}

var genericProductionFileNames = map[string]struct{}{
	"commands.go": {},
	"service.go":  {},
	"facade.go":   {},
	"compat.go":   {},
	"helpers.go":  {},
	"utils.go":    {},
	"types.go":    {},
	"ports.go":    {},
}

func TestProductionGenericFileNamesAreFrozen(t *testing.T) {
	repository := architectureRepository(t)
	for _, file := range repository.Files {
		if isTestFile(file.Path) {
			continue
		}
		name := filepath.Base(file.Path)
		if _, forbidden := genericProductionFileNames[name]; !forbidden {
			continue
		}
		if !genericProductionFileAllowed(file.Path) {
			t.Errorf("new generic production file %s; move the owner before adding it", file.Path)
		}
	}
}

func genericProductionFileAllowed(path string) bool {
	name := filepath.Base(path)
	if _, forbidden := genericProductionFileNames[name]; !forbidden {
		return true
	}
	return genericProductionFileAllowlist[path] != ""
}

var legacyMainFileAllowlist = map[string]string{
	modulePath + "/scripts/internal/releasenotesrender": "Task 19",
}

func TestProductionPackagesHaveStableMainFileNames(t *testing.T) {
	repository := architectureRepository(t)
	violations := make([]string, 0)
	for importPath, packageInfo := range repository.Graph.Packages {
		if !isScannedPackage(importPath) || len(packageInfo.GoFiles)+len(packageInfo.CgoFiles) == 0 {
			continue
		}
		relativeDirectory := packageRelativeDirectory(importPath)
		if packageInfo.Name == "main" {
			if !containsString(append(append([]string{}, packageInfo.GoFiles...), packageInfo.CgoFiles...), "main.go") {
				violations = append(violations, relativeDirectory+" missing main.go")
			}
			continue
		}
		want := filepath.Base(relativeDirectory) + ".go"
		files := append(append([]string{}, packageInfo.GoFiles...), packageInfo.CgoFiles...)
		if containsString(files, want) {
			continue
		}
		if task := legacyMainFileAllowlist[importPath]; task == "" {
			violations = append(violations, relativeDirectory+" missing "+want)
		}
	}
	sort.Strings(violations)
	for _, violation := range violations {
		t.Errorf("production package main file must match directory: %s", violation)
	}
}

func isScannedPackage(importPath string) bool {
	return importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
}

func packageRelativeDirectory(importPath string) string {
	if importPath == modulePath {
		return "."
	}
	return strings.TrimPrefix(importPath, modulePath+"/")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestStableProcessAndScriptEntrypoints(t *testing.T) {
	repository := architectureRepository(t)
	for importPath, packageInfo := range repository.Graph.Packages {
		if packageInfo.Name != "main" || len(packageInfo.GoFiles)+len(packageInfo.CgoFiles) == 0 {
			continue
		}
		relativeDirectory := packageRelativeDirectory(importPath)
		switch {
		case strings.HasPrefix(relativeDirectory, "cmd/"):
			if len(strings.Split(relativeDirectory, "/")) != 2 || !containsString(append(append([]string{}, packageInfo.GoFiles...), packageInfo.CgoFiles...), "main.go") {
				t.Errorf("cmd entrypoint must be cmd/<name>/main.go: %s", relativeDirectory)
			}
		case strings.HasPrefix(relativeDirectory, "scripts/"):
			parts := strings.Split(relativeDirectory, "/")
			if len(parts) != 2 || !containsString(append(append([]string{}, packageInfo.GoFiles...), packageInfo.CgoFiles...), "main.go") {
				t.Errorf("script entrypoint must keep scripts/<tool>/main.go: %s", relativeDirectory)
			}
		default:
			t.Errorf("production main package must live under cmd or scripts: %s", relativeDirectory)
		}
	}

	for _, file := range repository.Files {
		if isTestFile(file.Path) || !hasMainFunction(file.AST) {
			continue
		}
		if !strings.HasSuffix(file.Path, "/main.go") {
			t.Errorf("main function must live in a stable main.go entrypoint: %s", file.Path)
		}
	}

	cmdPackage := repository.Graph.Packages[modulePath+"/cmd/pixiv"]
	for _, imported := range cmdPackage.Imports {
		if isInternalImport(imported) && imported != modulePath+"/internal/cli" {
			t.Errorf("cmd/pixiv must delegate to internal/cli, found direct import %s", imported)
		}
	}
}

func hasMainFunction(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		function, ok := node.(*ast.FuncDecl)
		if ok && function.Recv == nil && function.Name.Name == "main" && function.Type.Params != nil && len(function.Type.Params.List) == 0 {
			found = true
			return false
		}
		return true
	})
	return found
}

func TestDependencyDirectionUsesOneGraph(t *testing.T) {
	repository := architectureRepository(t)
	imports := repository.productionImports()
	violations := make([]string, 0)
	for source, importedSet := range imports {
		for imported := range importedSet {
			reason := forbiddenDependency(source, imported)
			if reason == "" {
				continue
			}
			key := source + " -> " + imported
			if dependencyAllowlist[key] == "" {
				violations = append(violations, key+" ("+reason+")")
			}
		}
	}
	sort.Strings(violations)
	for _, violation := range violations {
		t.Errorf("new dependency-direction violation: %s", violation)
	}
}

func TestCLICompositionRootOwnsInfrastructureWiring(t *testing.T) {
	repository := architectureRepository(t)
	for _, file := range repository.filesInPackage("internal/cli", false) {
		for _, spec := range file.AST.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil || !isCLICompositionInfrastructure(imported) {
				continue
			}
			if file.Path != "internal/cli/cli.go" && file.Path != "internal/cli/root.go" {
				t.Errorf("CLI infrastructure wiring must stay in the private composition root: %s imports %s", file.Path, imported)
			}
		}
	}
}

// Task 12：generic pagination 只处理 Cursor 算法，bookmark/search workflow 不得
// 借由 application facade 重新取得 owner。更细的 Cursor 语义由 pagination 的
// external fake-cursor contract 覆盖。
func TestPaginationAndSearchRemainOutsideApplication(t *testing.T) {
	repository := architectureRepository(t)
	imports := repository.productionImports()
	for imported := range imports[modulePath+"/internal/pagination"] {
		if hasPrefixPath(imported, modulePath+"/sdk") {
			t.Errorf("internal/pagination must not import product SDK package %s", imported)
		}
	}
	for imported := range imports[modulePath+"/internal/search/pixiv"] {
		if hasPrefixPath(imported, modulePath+"/internal/application") {
			t.Errorf("internal/search/pixiv must not import application owner %s", imported)
		}
	}
}

// dependencyAllowlist 是迁移期间已经存在的边，而不是新的架构许可。边被真正
// 迁出后，必须连同对应 Task 编号一起删除；这样同一旧包新增一条反向边也会失败。
var dependencyAllowlist = map[string]string{}

func forbiddenDependency(source, imported string) string {
	switch {
	case source == modulePath+"/cmd/pixiv" && isInternalImport(imported) && imported != modulePath+"/internal/cli":
		return "process entry must delegate through internal/cli"
	case source == modulePath+"/internal/cli" && isCLICompositionInfrastructure(imported):
		return ""
	case hasPrefixPath(source, modulePath+"/internal/cli") && isCLICommandOwnerStorageAccess(source, imported):
		return ""
	case hasPrefixPath(source, modulePath+"/internal/cli") && hasAnyPrefix(imported,
		modulePath+"/internal/application", modulePath+"/internal/bootstrap", modulePath+"/internal/mcpserver",
		modulePath+"/internal/network", modulePath+"/internal/persistence", modulePath+"/internal/platform",
		modulePath+"/internal/services", modulePath+"/internal/storage"):
		return "CLI adapters must not depend directly on protocol/infrastructure or migration facades"
	case hasPrefixPath(source, modulePath+"/internal/mcpserver") && imported == modulePath+"/internal/cli/pipeline":
		return ""
	case hasPrefixPath(source, modulePath+"/internal/mcpserver") && hasAnyPrefix(imported,
		modulePath+"/internal/application", modulePath+"/internal/bootstrap", modulePath+"/internal/cli",
		modulePath+"/internal/network", modulePath+"/internal/persistence",
		modulePath+"/internal/platform", modulePath+"/internal/services", modulePath+"/internal/storage"):
		return "MCP adapters must depend on SDK/workflow ports, not migration or protocol packages"
	case hasPrefixPath(source, modulePath+"/internal/application") && hasAnyPrefix(imported,
		modulePath+"/internal/browsercookies", modulePath+"/internal/bootstrap", modulePath+"/internal/cli",
		modulePath+"/internal/media", modulePath+"/internal/mcpserver",
		modulePath+"/internal/network", modulePath+"/internal/persistence", modulePath+"/internal/platform",
		modulePath+"/internal/services", modulePath+"/internal/storage"):
		return "application use cases must not own media, protocol, storage, platform, or adapter code"
	case hasPrefixPath(source, modulePath+"/internal/services") && hasAnyPrefix(imported,
		modulePath+"/internal/application", modulePath+"/internal/bootstrap", modulePath+"/internal/cli",
		modulePath+"/internal/mcpserver", modulePath+"/internal/persistence", modulePath+"/internal/storage"):
		return "services must not reverse-depend on application, storage, or adapters"
	case hasPrefixPath(source, modulePath+"/sdk") && (imported == modulePath+"/internal/storage/file/atomic" ||
		imported == modulePath+"/internal/storage/file/secret"):
		// 公开 SDK 的 resource 保存使用协议无关的原子/私密文件机制。
		return ""
	case hasPrefixPath(source, modulePath+"/sdk") && hasAnyPrefix(imported,
		modulePath+"/internal/application", modulePath+"/internal/bootstrap", modulePath+"/internal/cli",
		modulePath+"/internal/mcpserver", modulePath+"/internal/persistence", modulePath+"/internal/storage"):
		return "public SDK must not expose application/composition/adapters"
	case hasPrefixPath(source, modulePath+"/internal/utils") && hasAnyPrefix(imported,
		modulePath+"/internal/application", modulePath+"/internal/cli",
		modulePath+"/internal/mcpserver", modulePath+"/internal/persistence", modulePath+"/internal/services",
		modulePath+"/internal/storage"):
		return "protocol-neutral utils must not import owners or adapters"
	case strings.HasPrefix(source, modulePath+"/scripts/") && hasAnyPrefix(imported,
		modulePath+"/internal/application", modulePath+"/internal/bootstrap", modulePath+"/internal/cli",
		modulePath+"/internal/mcpserver", modulePath+"/internal/services"):
		return "scripts must not own application or protocol wiring"
	}
	return ""
}

func isCLICompositionInfrastructure(imported string) bool {
	switch imported {
	case modulePath + "/internal/mcpserver/pixiv",
		modulePath + "/internal/mcpserver/fanbox",
		modulePath + "/internal/mcpserver/stdio",
		modulePath + "/internal/network",
		modulePath + "/internal/platform/localstate",
		modulePath + "/internal/storage/config",
		modulePath + "/internal/storage/database",
		modulePath + "/internal/storage/file/secret",
		modulePath + "/internal/storage/migration":
		return true
	default:
		return false
	}
}

// isCLICommandOwnerStorageAccess 识别 app-level 命令 owner 对 storage/platform
// 领域的合法依赖：这些命令自身就是对应领域的 owner（config 命令读写 config、
// auth 浏览器登录持久化 localstate/file 状态、FANBOX 命令读取 runtime config），
// 不属于迁移期需要清理的 composition seam。
func isCLICommandOwnerStorageAccess(source, imported string) bool {
	switch {
	case source == modulePath+"/internal/cli/config" && imported == modulePath+"/internal/storage/config":
		return true
	case source == modulePath+"/internal/cli/update" && imported == modulePath+"/internal/storage/config":
		return true
	case source == modulePath+"/internal/cli/internal/fanboxdeps" && imported == modulePath+"/internal/storage/config":
		return true
	case source == modulePath+"/internal/cli/pixiv/auth" && imported == modulePath+"/internal/storage/config":
		return true
	case source == modulePath+"/internal/cli/pixiv/auth/loginhelper" && (imported == modulePath+"/internal/platform/localstate" ||
		imported == modulePath+"/internal/storage/file/lock" || imported == modulePath+"/internal/storage/file/secret"):
		return true
	default:
		return false
	}
}

func hasAnyPrefix(value string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if hasPrefixPath(value, prefix) {
			return true
		}
	}
	return false
}

func TestCLIChildrenDoNotImportCLIOrMCPRoot(t *testing.T) {
	repository := architectureRepository(t)
	imports := repository.productionImports()
	for source, importedSet := range imports {
		if !hasPrefixPath(source, modulePath+"/internal/cli") || source == modulePath+"/internal/cli" {
			continue
		}
		if _, ok := importedSet[modulePath+"/internal/cli"]; ok {
			t.Errorf("CLI child %s must not import root package", strings.TrimPrefix(source, modulePath+"/"))
		}
	}
}

func TestCommandConstructorsDeclareInputSpec(t *testing.T) {
	repository := architectureRepository(t)
	violations := make([]string, 0)
	for _, file := range repository.Files {
		if isTestFile(file.Path) || !hasPrefixPath(packagePath(file.Path), modulePath+"/internal/cli") || packagePath(file.Path) == modulePath+"/internal/cli" {
			continue
		}
		for _, command := range cobraCommands(file) {
			if command.path == "internal/cli/root.go" {
				continue
			}
			if command.binding == "" {
				violations = append(violations, fmt.Sprintf("%s:%d (%s)", file.Path, file.FSet.Position(command.literal.Pos()).Line, command.use))
			}
		}
	}
	sort.Strings(violations)
	for _, violation := range violations {
		t.Errorf("command must bind an explicit pipeline.InputSpec: %s", violation)
	}
}

type cobraCommand struct {
	literal *ast.CompositeLit
	use     string
	path    string
	binding string
}

func cobraCommands(file sourceFile) []cobraCommand {
	functions := functionNodes(file.AST)
	commands := make([]cobraCommand, 0)
	ast.Inspect(file.AST, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || !isCobraCommandType(literal.Type) {
			return true
		}
		use := commandUse(literal)
		if use == "" {
			return true
		}
		owner := smallestFunctionContaining(functions, literal.Pos(), literal.End())
		binding := ""
		if owner != nil {
			binding = commandBinding(owner, literal.Pos())
		}
		commands = append(commands, cobraCommand{literal: literal, use: use, path: file.Path, binding: binding})
		return true
	})
	return commands
}

func isCobraCommandType(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if star, starOK := expression.(*ast.StarExpr); starOK {
		selector, ok = star.X.(*ast.SelectorExpr)
	}
	return ok && selector.Sel.Name == "Command" && selectorPackage(selector.X) == "cobra"
}

func commandUse(literal *ast.CompositeLit) string {
	for _, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok || selectorName(field.Key) != "Use" {
			continue
		}
		value, ok := field.Value.(*ast.BasicLit)
		if !ok || value.Kind != token.STRING {
			return "<dynamic>"
		}
		unquoted, err := strconv.Unquote(value.Value)
		if err != nil {
			return "<invalid>"
		}
		return unquoted
	}
	return ""
}

type functionNode struct {
	node ast.Node
	name string
}

func functionNodes(file *ast.File) []functionNode {
	functions := make([]functionNode, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		switch function := node.(type) {
		case *ast.FuncDecl:
			functions = append(functions, functionNode{node: function, name: function.Name.Name})
		case *ast.FuncLit:
			functions = append(functions, functionNode{node: function, name: "<literal>"})
		}
		return true
	})
	return functions
}

func smallestFunctionContaining(functions []functionNode, start, end token.Pos) *functionNode {
	var result *functionNode
	for index := range functions {
		candidate := &functions[index]
		if candidate.node.Pos() > start || candidate.node.End() < end {
			continue
		}
		if result == nil || candidate.node.End()-candidate.node.Pos() < result.node.End()-result.node.Pos() {
			result = candidate
		}
	}
	return result
}

func commandBinding(owner *functionNode, literalPos token.Pos) string {
	commandName := commandVariable(owner.node, literalPos)
	if commandName == "" {
		return ""
	}
	var binding string
	ast.Inspect(owner.node, func(node ast.Node) bool {
		if binding != "" {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 || !isIdent(call.Args[0], commandName) {
			return true
		}
		if isDirectInputSpecBind(call) {
			binding = "pipeline.Bind"
			return false
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !knownInputBindingMethod(selector.Sel.Name) {
			return true
		}
		binding = selector.Sel.Name
		return false
	})
	return binding
}

func commandVariable(owner ast.Node, literalPos token.Pos) string {
	var name string
	ast.Inspect(owner, func(node ast.Node) bool {
		if name != "" {
			return false
		}
		switch declaration := node.(type) {
		case *ast.AssignStmt:
			for index, rhs := range declaration.Rhs {
				if !containsPosition(rhs, literalPos) || index >= len(declaration.Lhs) {
					continue
				}
				if ident, ok := declaration.Lhs[index].(*ast.Ident); ok {
					name = ident.Name
				}
			}
		case *ast.ValueSpec:
			for index, value := range declaration.Values {
				if !containsPosition(value, literalPos) || index >= len(declaration.Names) {
					continue
				}
				name = declaration.Names[index].Name
			}
		}
		return true
	})
	return name
}

func containsPosition(node ast.Node, position token.Pos) bool {
	return node != nil && node.Pos() <= position && position <= node.End()
}

func isIdent(expression ast.Expr, want string) bool {
	ident, ok := expression.(*ast.Ident)
	return ok && ident.Name == want
}

func isDirectInputSpecBind(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Bind" || selectorPackage(selector.X) != "pipeline" || len(call.Args) < 2 {
		return false
	}
	composite, ok := call.Args[1].(*ast.CompositeLit)
	if !ok {
		return false
	}
	return selectorName(composite.Type) == "InputSpec"
}

func knownInputBindingMethod(name string) bool {
	switch name {
	case "bindNoInput", "bindTextValue", "bindTextValueWhen", "bindTextOrRecord",
		"BindNoInput", "BindTextValue", "BindTextValueWhen", "BindTextOrRecord":
		return true
	default:
		return false
	}
}

func selectorPackage(expression ast.Expr) string {
	ident, ok := expression.(*ast.Ident)
	if ok {
		return ident.Name
	}
	return ""
}

func selectorName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

func TestDTOOnlySerializationSinks(t *testing.T) {
	repository := architectureRepository(t)
	violations := make([]string, 0)
	for _, file := range repository.Files {
		if isTestFile(file.Path) || !isAdapterPath(file.Path) {
			continue
		}
		for _, call := range serializationCalls(file) {
			key := fmt.Sprintf("%s:%d", file.Path, file.FSet.Position(call.Pos()).Line)
			if serializationAllowlist[key] != "" || safeSerializationCallIn(repository, file, call) {
				continue
			}
			violations = append(violations, key)
		}
	}
	for _, violation := range violations {
		t.Errorf("CLI/MCP serialization must receive a DTO, pipeline Record, envelope, or scalar: %s", violation)
	}

	violations = mcpOutputViolations(repository)
	for _, violation := range violations {
		t.Errorf("MCP output type must not expose runtime entity or transport metadata: %s", violation)
	}
}

func isAdapterPath(path string) bool {
	path = filepath.ToSlash(path)
	// pipeline 是 Record/input mechanism owner；internal/* 下的 listing/deps 是
	// 通用 presenter/narrow-factory mechanism，其 DTO-only 契约在调用点由本门禁
	// 的 outputExpressionClass 检查。loginhelper 是本机 login-relay 协议 leaf。
	if hasPrefixPath(path, "internal/cli/pipeline") ||
		hasPrefixPath(path, "internal/cli/internal") ||
		hasPrefixPath(path, "internal/cli/pixiv/internal") ||
		hasPrefixPath(path, "internal/cli/fanbox/internal") ||
		hasPrefixPath(path, "internal/cli/pixiv/auth/loginhelper") {
		return false
	}
	return hasPrefixPath(path, "internal/cli") || hasPrefixPath(path, "internal/mcpserver")
}

func serializationCalls(file sourceFile) []*ast.CallExpr {
	calls := make([]*ast.CallExpr, 0)
	ast.Inspect(file.AST, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := selector.Sel.Name
		if name == "Encode" && !looksLikeJSONEncoder(selector.X) {
			return true
		}
		if name == "Encode" || name == "Marshal" || name == "MarshalIndent" || name == "PrintJSON" || name == "printJSON" || name == "MarshalJSONValue" || name == "MarshalSafeJSONValue" {
			if name == "Marshal" || name == "MarshalIndent" {
				if selectorPackage(selector.X) != "json" {
					return true
				}
			}
			calls = append(calls, call)
		}
		return true
	})
	return calls
}

func looksLikeJSONEncoder(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.CallExpr:
		selector, ok := value.Fun.(*ast.SelectorExpr)
		return ok && selectorPackage(selector.X) == "json" && selector.Sel.Name == "NewEncoder"
	case *ast.Ident:
		name := strings.ToLower(value.Name)
		return strings.Contains(name, "enc")
	default:
		return false
	}
}

func safeSerializationCall(file sourceFile, call *ast.CallExpr) bool {
	return safeSerializationCallIn(nil, file, call)
}

// safeSerializationCallIn 判定一次序列化调用是否收到 DTO/Record/scalar。
// repository 非空时对跨文件本地转换函数返回类型做完整解析（例如 account.go
// 的 accountOutFromResult 被 login.go 调用）；否则仅在当前文件内解析（fixture）。
func safeSerializationCallIn(repository *sourceRepository, file sourceFile, call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name := selector.Sel.Name
	if name == "Marshal" || name == "MarshalIndent" {
		if len(call.Args) == 0 {
			return false
		}
		return safeScalarExpression(call.Args[0]) || safeGenericSeamArgument(file, call, call.Args[0])
	}
	if name == "Encode" {
		return len(call.Args) > 0 && outputExpressionClassIn(file, repository, call, call.Args[0]) != outputRuntime
	}
	if name == "PrintJSON" || name == "printJSON" {
		if len(call.Args) == 0 {
			return false
		}
		switch outputExpressionClassIn(file, repository, call, call.Args[0]) {
		case outputDTO, outputRecord, outputScalar:
			return true
		default:
			return false
		}
	}
	return false
}

// safeGenericSeamArgument 识别窄 JSON host 方法（printJSON/WriteJSON/
// marshalJSONValue/writeJSONLine 等）内部对 any 参数的序列化。这些 seam 的
// 契约是调用方必须先转换 DTO/Record/envelope；门禁继续检查 seam 的调用点。
func safeGenericSeamArgument(file sourceFile, call *ast.CallExpr, expression ast.Expr) bool {
	ident, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	owner := smallestFunctionContaining(functionNodes(file.AST), call.Pos(), call.End())
	if owner == nil {
		return false
	}
	var function *ast.FuncDecl
	switch value := owner.node.(type) {
	case *ast.FuncDecl:
		function = value
	case *ast.FuncLit:
		return false
	default:
		return false
	}
	if function.Type == nil || function.Type.Params == nil {
		return false
	}
	for _, field := range function.Type.Params.List {
		if !containsIdentName(field.Names, ident.Name) {
			continue
		}
		return isAnyType(field.Type)
	}
	return false
}

func containsIdentName(names []*ast.Ident, want string) bool {
	for _, name := range names {
		if name.Name == want {
			return true
		}
	}
	return false
}

func isAnyType(expression ast.Expr) bool {
	ident, ok := expression.(*ast.Ident)
	return ok && (ident.Name == "any" || ident.Name == "interface{}")
}

func safeScalarExpression(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return value.Name == "extraName" || value.Name == "extraValue"
	default:
		return false
	}
}

type outputClass uint8

const (
	outputUnknown outputClass = iota
	outputRuntime
	outputDTO
	outputRecord
	outputScalar
)

func outputExpressionClass(file sourceFile, call *ast.CallExpr, expression ast.Expr) outputClass {
	return outputExpressionClassIn(file, nil, call, expression)
}

func outputExpressionClassIn(file sourceFile, repository *sourceRepository, call *ast.CallExpr, expression ast.Expr) outputClass {
	if safeScalarExpression(expression) {
		return outputScalar
	}
	if safeGenericSeamArgument(file, call, expression) {
		return outputDTO
	}
	switch value := expression.(type) {
	case *ast.Ident:
		if value.Name == "record" || strings.HasSuffix(value.Name, "Record") {
			return outputRecord
		}
		if isDTOTypeName(value.Name) {
			return outputDTO
		}
		if inferred := declaredValueClassIn(file, repository, value.Name); inferred != outputUnknown {
			return inferred
		}
	case *ast.CompositeLit:
		name := selectorName(value.Type)
		if name == "" {
			if compositeContainsRuntimeType(value) {
				return outputRuntime
			}
			return outputDTO
		}
		if isDTOTypeName(name) {
			return outputDTO
		}
	case *ast.CallExpr:
		name := selectorName(value.Fun)
		if name == "SafeJSONValue" || strings.HasSuffix(name, "DTO") || strings.HasSuffix(name, "Out") || strings.HasSuffix(name, "OutFrom") || strings.HasPrefix(name, "RecordFrom") {
			return outputDTO
		}
		if result := localCallReturnClassIn(file, repository, name); result != outputUnknown {
			return result
		}
	}
	return outputRuntime
}

// isDTOTypeName 识别显式输出 DTO/envelope 命名。adapter 内的 *Response 类型是
// 协议输出信封，不属于 runtime entity。
func isDTOTypeName(name string) bool {
	switch {
	case strings.HasSuffix(name, "DTO"),
		strings.HasSuffix(name, "Out"),
		strings.HasSuffix(name, "Envelope"),
		strings.HasSuffix(name, "Report"),
		strings.HasSuffix(name, "Response"),
		// 最终 product DTO：update check 报告与 version JSON 输出。
		name == "UpdateResult",
		name == "Info":
		return true
	default:
		return false
	}
}

func compositeContainsRuntimeType(literal *ast.CompositeLit) bool {
	found := false
	ast.Inspect(literal, func(node ast.Node) bool {
		if found {
			return false
		}
		selector, ok := node.(*ast.SelectorExpr)
		if ok && isRuntimeEntityType(selectorPackage(selector.X), selector.Sel.Name) {
			found = true
		}
		return true
	})
	return found
}

// declaredValueClass 解析局部变量声明的输出类别，支持 := 赋值、显式声明、
// range 变量（元素类别）与本地转换函数返回值。
func declaredValueClass(file sourceFile, name string) outputClass {
	return declaredValueClassIn(file, nil, name)
}

// declaredValueClassIn 解析局部变量声明的输出类别。repository 非空时对跨文件
// 本地转换函数返回类型做完整解析；否则仅在当前文件内解析（fixture 测试）。
func declaredValueClassIn(file sourceFile, repository *sourceRepository, name string) outputClass {
	var result outputClass
	ast.Inspect(file.AST, func(node ast.Node) bool {
		if result != outputUnknown {
			return false
		}
		switch declaration := node.(type) {
		case *ast.ValueSpec:
			for index, declared := range declaration.Names {
				if declared.Name != name || index >= len(declaration.Values) {
					continue
				}
				result = expressionClassFromSyntaxIn(file, repository, declaration.Type, declaration.Values[index])
			}
		case *ast.AssignStmt:
			for index, lhs := range declaration.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name != name || index >= len(declaration.Rhs) {
					continue
				}
				result = expressionClassFromSyntaxIn(file, repository, nil, declaration.Rhs[index])
			}
		case *ast.RangeStmt:
			element := declaration.Value
			if element == nil {
				element = declaration.Key
			}
			if ident, ok := element.(*ast.Ident); ok && ident.Name == name {
				result = rangeElementClass(file, declaration.X)
			}
		}
		return true
	})
	return result
}

// rangeElementClass 从 range 源推导元素类别：标识符回溯声明，selector 链无法在
// AST-only 门禁内解析，返回 unknown 由调用点继续判定。
func rangeElementClass(file sourceFile, source ast.Expr) outputClass {
	ident, ok := source.(*ast.Ident)
	if !ok {
		return outputUnknown
	}
	return declaredValueClass(file, ident.Name)
}

// localCallReturnClass 解析同文件本地函数声明的第一个返回值类型；覆盖
// converter 函数（如 accountOutFromResult）返回 DTO 的常见模式。
func localCallReturnClass(file sourceFile, name string) outputClass {
	return localCallReturnClassIn(file, nil, name)
}

// localCallReturnClassIn 解析本地函数声明的第一个返回值类型。repository 非空时
// 跨文件搜索同名本地函数（例如 account.go 定义的 accountOutFromResult 被
// login.go 调用），否则只在当前文件内解析（fixture 测试使用）。
func localCallReturnClassIn(file sourceFile, repository *sourceRepository, name string) outputClass {
	if !isLocalCallName(name) {
		return outputUnknown
	}
	files := []sourceFile{file}
	if repository != nil {
		files = repository.Files
	}
	var result outputClass
	for _, candidate := range files {
		ast.Inspect(candidate.AST, func(node ast.Node) bool {
			if result != outputUnknown {
				return false
			}
			function, ok := node.(*ast.FuncDecl)
			if !ok || function.Name.Name != name || function.Type == nil || function.Type.Results == nil || len(function.Type.Results.List) == 0 {
				return true
			}
			result = typeNameClass(function.Type.Results.List[0].Type)
			return true
		})
		if result != outputUnknown {
			break
		}
	}
	return result
}

func isLocalCallName(name string) bool {
	if name == "" {
		return false
	}
	first := name[0]
	return first >= 'a' && first <= 'z' || first >= 'A' && first <= 'Z'
}

func typeNameClass(expression ast.Expr) outputClass {
	switch value := expression.(type) {
	case *ast.Ident:
		if value.Name == "Record" {
			return outputRecord
		}
		if isDTOTypeName(value.Name) {
			return outputDTO
		}
	case *ast.SelectorExpr:
		if isDTOTypeName(value.Sel.Name) {
			return outputDTO
		}
	case *ast.StarExpr:
		return typeNameClass(value.X)
	}
	return outputUnknown
}

func expressionClassFromSyntax(file sourceFile, declared ast.Expr, value ast.Expr) outputClass {
	return expressionClassFromSyntaxIn(file, nil, declared, value)
}

func expressionClassFromSyntaxIn(file sourceFile, repository *sourceRepository, declared ast.Expr, value ast.Expr) outputClass {
	if declared != nil {
		if result := typeNameClass(declared); result != outputUnknown {
			return result
		}
		if safeScalarExpression(declared) {
			return outputScalar
		}
	}
	if call, ok := value.(*ast.CallExpr); ok {
		name := selectorName(call.Fun)
		if strings.HasSuffix(name, "DTO") || strings.HasSuffix(name, "Out") || strings.HasSuffix(name, "OutFrom") || strings.HasPrefix(name, "RecordFrom") {
			return outputDTO
		}
		if name == "make" && len(call.Args) > 0 {
			if array, ok := call.Args[0].(*ast.ArrayType); ok {
				return typeNameClass(array.Elt)
			}
		}
		if result := localCallReturnClassIn(file, repository, name); result != outputUnknown {
			return result
		}
	}
	if literal, ok := value.(*ast.CompositeLit); ok {
		if result := typeNameClass(literal.Type); result != outputUnknown {
			return result
		}
	}
	return outputUnknown
}

var serializationAllowlist = map[string]string{}

func mcpOutputViolations(repository *sourceRepository) []string {
	violations := make([]string, 0)
	for _, file := range repository.Files {
		if isTestFile(file.Path) || !hasPrefixPath(file.Path, "internal/mcpserver") {
			continue
		}
		ast.Inspect(file.AST, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok || !strings.HasSuffix(spec.Name.Name, "Out") {
				return true
			}
			structure, ok := spec.Type.(*ast.StructType)
			if !ok {
				return true
			}
			if outputStructAllowlisted(file.Path, spec.Name.Name) {
				return true
			}
			for _, field := range structure.Fields.List {
				fieldName := ""
				if len(field.Names) > 0 {
					fieldName = field.Names[0].Name
				}
				if unsafeOutputFieldName(fieldName) {
					violations = append(violations, fmt.Sprintf("%s:%d %s.%s", file.Path, file.FSet.Position(field.Pos()).Line, spec.Name.Name, fieldName))
				}
				pkg, typeName := selectorTypeName(field.Type)
				if isRuntimeEntityType(pkg, typeName) {
					violations = append(violations, fmt.Sprintf("%s:%d %s.%s uses %s.%s", file.Path, file.FSet.Position(field.Pos()).Line, spec.Name.Name, fieldName, pkg, typeName))
				}
			}
			return true
		})
	}
	sort.Strings(violations)
	return uniqueStrings(violations)
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func outputStructAllowlisted(path, name string) bool {
	return mcpOutputStructAllowlist[path+":"+name] != ""
}

var mcpOutputStructAllowlist = map[string]string{}

func unsafeOutputFieldName(name string) bool {
	switch name {
	case "Cookie", "Cookies", "Expiry", "Headers", "Locator", "RequestHeaders", "RefreshToken", "Token":
		return true
	default:
		return false
	}
}

func selectorTypeName(expression ast.Expr) (string, string) {
	switch value := expression.(type) {
	case *ast.StarExpr:
		return selectorTypeName(value.X)
	case *ast.ArrayType:
		return selectorTypeName(value.Elt)
	case *ast.MapType:
		return selectorTypeName(value.Value)
	case *ast.SelectorExpr:
		return selectorPackage(value.X), value.Sel.Name
	case *ast.Ident:
		return "", value.Name
	default:
		return "", ""
	}
}

func isRuntimeEntityType(pkg, name string) bool {
	if pkg != "pixiv" && pkg != "fanbox" && pkg != "fanboxsdk" {
		return false
	}
	switch name {
	case "Artwork", "Comment", "CommentPage", "Creator", "CreatorSummary", "ImageResource", "Novel", "NovelContent", "NovelSeries", "Post", "Resource", "Tag", "TrendingTag", "User", "UserDetail", "UserPreview":
		return true
	default:
		return false
	}
}

func TestNamespaceRootsDoNotGrowFacades(t *testing.T) {
	repository := architectureRepository(t)
	namespaces := []string{"internal/account", "internal/media", "internal/mcpserver", "internal/services", "internal/storage"}
	for _, namespace := range namespaces {
		root := filepath.Join(repository.Root, filepath.FromSlash(namespace))
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || isTestFile(entry.Name()) {
				continue
			}
			path := namespace + "/" + entry.Name()
			if task := namespaceFacadeAllowlist[path]; task == "" {
				t.Errorf("namespace %s must not contain a root facade: %s", namespace, path)
			}
		}
	}
	for path, task := range namespaceFacadeAllowlist {
		if task == "" {
			t.Errorf("namespace facade allowlist entry %s has no deletion task", path)
		}
	}

	for _, namespace := range []string{"internal/cli"} {
		root := filepath.Join(repository.Root, filepath.FromSlash(namespace))
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || isTestFile(entry.Name()) {
				continue
			}
			path := namespace + "/" + entry.Name()
			if namespace == "internal/cli" {
				if _, ok := cliCompositionRootFiles[path]; ok {
					continue
				}
			}
			if task := legacyRootFacadeAllowlist[path]; task == "" {
				t.Errorf("migration root %s contains an untracked facade file: %s", namespace, path)
			}
		}
	}
	for path := range cliCompositionRootFiles {
		if _, err := os.Stat(filepath.Join(repository.Root, filepath.FromSlash(path))); err != nil {
			t.Errorf("CLI composition root is missing: %s", path)
		}
	}
}

var namespaceFacadeAllowlist = map[string]string{}

var legacyRootFacadeAllowlist = map[string]string{}

// cliCompositionRootFiles 是目标架构中持久存在的 private composition root，
// 不是迁移 facade；它可以保留 infrastructure wiring，但不得成为命令 owner。
var cliCompositionRootFiles = map[string]struct{}{
	"internal/cli/cli.go":  {},
	"internal/cli/root.go": {},
}

func TestExternalTestPolicy(t *testing.T) {
	repository := architectureRepository(t)
	productionPackageNames := make(map[string]string)
	for _, file := range repository.Files {
		if isTestFile(file.Path) {
			continue
		}
		productionPackageNames[filepath.ToSlash(filepath.Dir(file.Path))] = file.Name
	}
	for _, file := range repository.Files {
		if !isTestFile(file.Path) {
			continue
		}
		directory := filepath.ToSlash(filepath.Dir(file.Path))
		productionName, ok := productionPackageNames[directory]
		if !ok || file.Name != productionName {
			continue
		}
		if task := legacyInternalTestAllowlist[directory]; task == "" && !permanentSamePackageTest(directory) {
			t.Errorf("new behavior test must use external package %s: %s", productionName+"_test", file.Path)
		}
	}
}

// permanentSamePackageTest 识别目标架构中持久保留 same-package 测试的目录：
// `internal/cli` 是 private composition root，其集成测试必须同包替换资源工厂
// seam 并直接观察私有运行图，无法在 external package 内观察。
func permanentSamePackageTest(directory string) bool {
	return directory == "internal/cli"
}

// 这些是迁移开始前已有的 same-package tests。它们只能作为已知基线存在；新 owner
// 的测试必须使用 external package，后续 owner task 迁移完对应目录后删除整项。
var legacyInternalTestAllowlist = map[string]string{
	// Browser providers must observe private state (encryption key override,
	// profile discovery, binarycookies record layout), so their tests stay
	// same-package and observe the provider directly rather than exporting
	// internals. Remaining support-package tests are externalized.
	"internal/browsercookies/chromium": "Task 18",
	"internal/browsercookies/firefox":  "Task 18",
	"internal/browsercookies/safari":   "Task 18",
	"internal/browsercookies/secret":   "Task 18",
	// Platform installers and the release source-route seam inspect and inject
	// unexported installer/cache/sourceSelector state.
	"internal/update/installer":              "Task 18",
	"internal/update/release":                "Task 18",
	"scripts/internal/browsernativeevidence": "Task 19",
	"scripts/internal/changescope":           "Task 19",
	"scripts/internal/homebrewformula":       "Task 19",
	"scripts/internal/licensebundle":         "Task 19",
	"scripts/internal/linuxabi":              "Task 19",
	"scripts/internal/nativeevidence":        "Task 19",
	"scripts/internal/prepublishhomebrew":    "Task 19",
	"scripts/internal/publicapi":             "Task 19",
	"scripts/internal/releaseassets":         "Task 19",
	"scripts/internal/releasenotes":          "Task 19",
	"scripts/internal/releaseworkflow":       "Task 19",
	"scripts/internal/understandgraph":       "Task 19",
}

func TestArchitectureRuleFixtures(t *testing.T) {
	t.Run("input binding", func(t *testing.T) {
		cases := []struct {
			name string
			src  string
			want bool
		}{
			{name: "direct InputSpec", src: `package fixture
import "github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
import "github.com/spf13/cobra"
func command() *cobra.Command { cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}; pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.NoInput}); return cmd }`, want: true},
			{name: "named helper", src: `package fixture
import "github.com/spf13/cobra"
type controller struct{}
func (controller) bindNoInput(*cobra.Command) {}
func (a controller) command() *cobra.Command { cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}; a.bindNoInput(cmd); return cmd }`, want: true},
			{name: "missing binding", src: `package fixture
import "github.com/spf13/cobra"
func command() *cobra.Command { return &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }} }`, want: false},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				fileSet := token.NewFileSet()
				file, err := parser.ParseFile(fileSet, testCase.name+".go", testCase.src, 0)
				if err != nil {
					t.Fatal(err)
				}
				commands := cobraCommands(sourceFile{Path: testCase.name + ".go", Name: "fixture", AST: file, FSet: fileSet})
				got := len(commands) == 1 && commands[0].binding != ""
				if got != testCase.want {
					t.Fatalf("binding=%t, want %t: %#v", got, testCase.want, commands)
				}
			})
		}
	})

	t.Run("types classify DTO and runtime", func(t *testing.T) {
		const source = `package fixture
type Artwork struct{}
type ArtworkDTO struct{}
var runtime Artwork
var dto ArtworkDTO
`
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, "types.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		info := &types.Info{
			Defs:  make(map[*ast.Ident]types.Object),
			Types: make(map[ast.Expr]types.TypeAndValue),
		}
		checked, err := (&types.Config{}).Check("fixture", fileSet, []*ast.File{file}, info)
		if err != nil {
			t.Fatal(err)
		}
		for _, testCase := range []struct {
			name string
			want outputClass
		}{
			{name: "runtime", want: outputRuntime},
			{name: "dto", want: outputDTO},
		} {
			var expression ast.Expr
			for _, declaration := range file.Decls {
				valueSpec, ok := declaration.(*ast.GenDecl)
				if !ok || len(valueSpec.Specs) != 1 {
					continue
				}
				value, ok := valueSpec.Specs[0].(*ast.ValueSpec)
				if ok && len(value.Names) == 1 && value.Names[0].Name == testCase.name {
					expression = value.Names[0]
				}
			}
			if expression == nil {
				t.Fatalf("fixture variable %s not found", testCase.name)
			}
			object := info.Defs[expression.(*ast.Ident)]
			if object == nil {
				t.Fatalf("fixture variable %s has no type object", testCase.name)
			}
			got := classifyGoType(object.Type(), checked)
			if got != testCase.want {
				t.Errorf("%s classified as %v, want %v", testCase.name, got, testCase.want)
			}
		}
	})

	t.Run("dependency direction and serialization", func(t *testing.T) {
		for _, testCase := range []struct {
			name     string
			source   string
			imported string
			wantBad  bool
		}{
			{name: "CLI to protocol", source: modulePath + "/internal/cli/pixiv", imported: modulePath + "/internal/services/pixiv/appapi", wantBad: true},
			{name: "service to CLI", source: modulePath + "/internal/services/pixiv/appapi", imported: modulePath + "/internal/cli", wantBad: true},
			{name: "CLI to public SDK", source: modulePath + "/internal/cli/pixiv", imported: modulePath + "/sdk/pixiv", wantBad: false},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				got := forbiddenDependency(testCase.source, testCase.imported) != ""
				if got != testCase.wantBad {
					t.Fatalf("forbiddenDependency=%t, want %t", got, testCase.wantBad)
				}
			})
		}

		const source = `package fixture
import "encoding/json"
type Artwork struct{}
type ArtworkDTO struct{}
func run() {
    dto := ArtworkDTO{}
    runtime := Artwork{}
    _ = json.NewEncoder(nil).Encode(dto)
    _ = json.NewEncoder(nil).Encode(runtime)
}`
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, "serialization.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		parsed := sourceFile{Path: "internal/cli/fixture.go", Name: "fixture", AST: file, FSet: fileSet}
		calls := make([]*ast.CallExpr, 0)
		for _, call := range serializationCalls(parsed) {
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Encode" {
				calls = append(calls, call)
			}
		}
		if len(calls) != 2 {
			t.Fatalf("got %d encoder calls, want 2", len(calls))
		}
		if !safeSerializationCall(parsed, calls[0]) {
			t.Fatalf("DTO fixture must be accepted as an output type")
		}
		if safeSerializationCall(parsed, calls[1]) {
			t.Fatalf("runtime fixture must be rejected as an output type")
		}
	})

	for _, testCase := range []struct {
		name string
		path string
		want bool
	}{
		{name: "current generic file is temporary", path: "internal/cli/pixiv/auth/service.go", want: false},
		{name: "new generic file is rejected", path: "internal/cli/pixiv/helpers.go", want: false},
		{name: "normal owner file is accepted", path: "internal/cli/pixiv/search.go", want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := genericProductionFileAllowed(testCase.path); got != testCase.want {
				t.Fatalf("genericProductionFileAllowed(%q)=%t, want %t", testCase.path, got, testCase.want)
			}
		})
	}

	t.Run("no retired package roots remain", func(t *testing.T) {
		if len(retiredPackageRoots()) != 0 {
			t.Fatalf("retired package roots remain: %v", retiredPackageRoots())
		}
		if _, ok := retiredPackageRoot(modulePath + "/internal/application/future"); ok {
			t.Fatalf("retired root unexpectedly covers internal/application/future")
		}
	})
}

func classifyGoType(typ types.Type, _ *types.Package) outputClass {
	if typ == nil {
		return outputUnknown
	}
	switch value := typ.(type) {
	case *types.Basic:
		return outputScalar
	case *types.Named:
		name := value.Obj().Name()
		if name == "Record" && value.Obj().Pkg() != nil && strings.HasSuffix(value.Obj().Pkg().Path(), "/internal/record") {
			return outputRecord
		}
		if strings.HasSuffix(name, "DTO") || strings.HasSuffix(name, "Out") || strings.HasSuffix(name, "Envelope") || strings.HasSuffix(name, "Report") {
			return outputDTO
		}
		return outputRuntime
	case *types.Slice:
		return classifyGoType(value.Elem(), nil)
	case *types.Pointer:
		return classifyGoType(value.Elem(), nil)
	default:
		return outputUnknown
	}
}

func TestArchitectureAllowlistEntriesHaveTaskNumbers(t *testing.T) {
	allowlists := []map[string]string{
		retiredDirectoryAllowlist,
		retiredPackageAllowlist,
		retiredFileAllowlist,
		genericProductionFileAllowlist,
		legacyMainFileAllowlist,
		dependencyAllowlist,
		serializationAllowlist,
		namespaceFacadeAllowlist,
		legacyRootFacadeAllowlist,
		legacyInternalTestAllowlist,
		mcpOutputStructAllowlist,
	}
	for _, allowlist := range allowlists {
		for key, task := range allowlist {
			if !strings.Contains(task, "Task") {
				t.Errorf("allowlist entry %s has no task reference: %s", key, task)
			}
		}
	}
}

func TestArchitectureAllowlistEntriesMatchCurrentState(t *testing.T) {
	repository := architectureRepository(t)
	files := make(map[string]sourceFile, len(repository.Files))
	for _, file := range repository.Files {
		files[file.Path] = file
	}

	for importPath := range retiredPackageAllowlist {
		if _, ok := repository.Graph.Packages[importPath]; !ok {
			t.Errorf("retired package allowlist entry is not a current package: %s", importPath)
		}
	}

	for path := range genericProductionFileAllowlist {
		file, ok := files[path]
		if !ok || isTestFile(path) {
			t.Errorf("generic file allowlist entry is not a production file: %s", path)
			continue
		}
		if _, forbidden := genericProductionFileNames[filepath.Base(path)]; !forbidden {
			t.Errorf("generic file allowlist entry has a non-generic filename: %s", path)
		}
		if file.Name == "" {
			t.Errorf("generic file allowlist entry has no parsed package: %s", path)
		}
	}

	for importPath := range legacyMainFileAllowlist {
		packageInfo, ok := repository.Graph.Packages[importPath]
		if !ok {
			t.Errorf("legacy main-file allowlist entry is not a current package: %s", importPath)
			continue
		}
		want := filepath.Base(packageRelativeDirectory(importPath)) + ".go"
		files := append(append([]string{}, packageInfo.GoFiles...), packageInfo.CgoFiles...)
		if containsString(files, want) {
			t.Errorf("legacy main-file allowlist entry is stale: %s already has %s", importPath, want)
		}
	}

	imports := repository.productionImports()
	for edge := range dependencyAllowlist {
		source, imported, ok := strings.Cut(edge, " -> ")
		if !ok || imports[source] == nil {
			t.Errorf("dependency allowlist entry has no current source package: %s", edge)
			continue
		}
		if _, ok := imports[source][imported]; !ok {
			t.Errorf("dependency allowlist entry has no current import edge: %s", edge)
			continue
		}
		if forbiddenDependency(source, imported) == "" {
			t.Errorf("dependency allowlist entry is no longer a forbidden migration edge: %s", edge)
		}
	}

	for key := range serializationAllowlist {
		path, rawLine, ok := strings.Cut(key, ":")
		if !ok {
			t.Errorf("serialization allowlist entry has no line number: %s", key)
			continue
		}
		line, err := strconv.Atoi(rawLine)
		if err != nil {
			t.Errorf("serialization allowlist entry has invalid line number: %s", key)
			continue
		}
		file, ok := files[path]
		if !ok || isTestFile(path) {
			t.Errorf("serialization allowlist entry is not a production file: %s", key)
			continue
		}
		found := false
		for _, call := range serializationCalls(file) {
			if file.FSet.Position(call.Pos()).Line == line {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("serialization allowlist entry has no current serialization sink: %s", key)
		}
	}

	for path := range namespaceFacadeAllowlist {
		if file, ok := files[path]; !ok || isTestFile(path) {
			t.Errorf("namespace facade allowlist entry is not a production file: %s", path)
		} else if file.Name == "" {
			t.Errorf("namespace facade allowlist entry has no parsed package: %s", path)
		}
	}

	for path := range legacyRootFacadeAllowlist {
		if file, ok := files[path]; !ok || isTestFile(path) {
			t.Errorf("legacy root facade allowlist entry is not a production file: %s", path)
		} else if file.Name == "" {
			t.Errorf("legacy root facade allowlist entry has no parsed package: %s", path)
		}
	}

	for directory := range legacyInternalTestAllowlist {
		productionFiles := repository.filesInPackage(directory, false)
		if len(productionFiles) == 0 {
			t.Errorf("legacy same-package test allowlist entry has no current production package: %s", directory)
			continue
		}
		productionName := productionFiles[0].Name
		found := false
		for _, file := range repository.filesInPackage(directory, true) {
			if file.Name == productionName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("legacy same-package test allowlist entry has no current same-package test: %s", directory)
		}
	}

	for key := range mcpOutputStructAllowlist {
		path, name, ok := strings.Cut(key, ":")
		if !ok {
			t.Errorf("MCP output allowlist entry has no type name: %s", key)
			continue
		}
		file, ok := files[path]
		if !ok || isTestFile(path) {
			t.Errorf("MCP output allowlist entry is not a production file: %s", key)
			continue
		}
		found := false
		ast.Inspect(file.AST, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if ok && spec.Name.Name == name {
				found = true
				return false
			}
			return true
		})
		if !found {
			t.Errorf("MCP output allowlist entry has no current type: %s", key)
		}
	}
}
