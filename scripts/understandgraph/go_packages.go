package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func readModulePath(root string) (string, error) {
	path := filepath.Join(root, "go.mod")
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open go.mod: %w", err)
	}
	defer file.Close()

	var modulePath string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "//", 2)[0])
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "module" {
			continue
		}
		if len(fields) != 2 || modulePath != "" {
			return "", fmt.Errorf("go.mod must contain exactly one valid module directive")
		}
		modulePath = fields[1]
		if unquoted, err := strconv.Unquote(modulePath); err == nil {
			modulePath = unquoted
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	if modulePath == "" {
		return "", fmt.Errorf("go.mod module directive is missing")
	}
	return modulePath, nil
}

func analyzeGoPackages(root, modulePath string, files []scanFile) ([]goSource, []goPackage, error) {
	sources := make([]goSource, 0)
	sourceIndexesByDir := map[string][]int{}
	for _, file := range files {
		if file.Language != "go" {
			continue
		}
		absolute := filepath.Join(root, filepath.FromSlash(file.Path))
		content, err := os.ReadFile(absolute)
		if err != nil {
			return nil, nil, fmt.Errorf("read Go source %s: %w", file.Path, err)
		}
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, absolute, content, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("parse Go source %s: %w", file.Path, err)
		}
		dir := filepath.ToSlash(filepath.Dir(file.Path))
		if dir == "." {
			dir = ""
		}
		imports := make([]string, 0, len(parsed.Imports))
		for _, spec := range parsed.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return nil, nil, fmt.Errorf("parse Go import in %s: %w", file.Path, err)
			}
			imports = append(imports, value)
		}
		functions := make([]goFunction, 0)
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			receiver, baseReceiver, err := receiverNames(fileSet, function)
			if err != nil {
				return nil, nil, fmt.Errorf("parse Go receiver in %s:%d: %w", file.Path, fileSet.Position(function.Pos()).Line, err)
			}
			qualified := function.Name.Name
			if baseReceiver != "" {
				qualified = baseReceiver + "." + function.Name.Name
			}
			functions = append(functions, goFunction{
				Name: function.Name.Name, Receiver: receiver, QualifiedName: qualified,
				StartLine: fileSet.Position(function.Pos()).Line,
				EndLine:   fileSet.Position(function.End()).Line,
			})
		}
		digest := sha256.Sum256(content)
		newlineCount := bytes.Count(content, []byte{'\n'})
		if file.SizeLines != newlineCount {
			return nil, nil, fmt.Errorf("scan sizeLines mismatch for %s: got %d, source has %d", file.Path, file.SizeLines, newlineCount)
		}
		sources = append(sources, goSource{
			Path: file.Path, PackageName: parsed.Name.Name, Imports: imports, Functions: functions,
			IsTest:      strings.HasSuffix(filepath.Base(file.Path), "_test.go"),
			ContentHash: fmt.Sprintf("%x", digest), TotalLines: newlineCount + 1,
		})
		sourceIndexesByDir[dir] = append(sourceIndexesByDir[dir], len(sources)-1)
	}

	primaryByDir := map[string]string{}
	for dir, indexes := range sourceIndexesByDir {
		sort.Slice(indexes, func(i, j int) bool { return sources[indexes[i]].Path < sources[indexes[j]].Path })
		primary := ""
		for _, index := range indexes {
			source := sources[index]
			if source.IsTest {
				continue
			}
			if primary != "" && primary != source.PackageName {
				return nil, nil, fmt.Errorf("Go directory %q contains multiple production packages", dir)
			}
			primary = source.PackageName
		}
		if primary == "" {
			primary = sources[indexes[0]].PackageName
			if strings.HasSuffix(primary, "_test") {
				primary = strings.TrimSuffix(primary, "_test")
			}
			if primary == "" {
				return nil, nil, fmt.Errorf("Go directory %q cannot infer package name from test-only files", dir)
			}
		}
		for _, index := range indexes {
			source := &sources[index]
			if !source.IsTest {
				continue
			}
			switch source.PackageName {
			case primary:
				source.ExternalTest = false
			case primary + "_test":
				source.ExternalTest = true
			default:
				return nil, nil, fmt.Errorf(
					"Go test source %s has package %s; want %s or %s",
					source.Path, source.PackageName, primary, primary+"_test",
				)
			}
		}
		primaryByDir[dir] = primary
	}

	packageByID := map[string]*goPackage{}
	primaryIDByImportPath := map[string]string{}
	for index := range sources {
		source := &sources[index]
		dir := filepath.ToSlash(filepath.Dir(source.Path))
		if dir == "." {
			dir = ""
		}
		importPath := modulePath
		if dir != "" {
			importPath += "/" + dir
		}
		id := "module:" + importPath
		if source.ExternalTest {
			// external test package 与被测 package 是不同编译单元，不能合并到同一 module。
			id += "#" + source.PackageName
		} else {
			primaryIDByImportPath[importPath] = id
		}
		source.PackageID = id
		pkg := packageByID[id]
		if pkg == nil {
			pkg = &goPackage{ID: id, Name: source.PackageName, ImportPath: importPath}
			packageByID[id] = pkg
		}
		pkg.Files = append(pkg.Files, source.Path)
	}

	for index := range sources {
		internal := make([]string, 0)
		for _, imported := range sources[index].Imports {
			if imported != modulePath && !strings.HasPrefix(imported, modulePath+"/") {
				continue
			}
			target, ok := primaryIDByImportPath[imported]
			if !ok {
				return nil, nil, fmt.Errorf("Go source %s imports missing internal package %s", sources[index].Path, imported)
			}
			internal = append(internal, target)
		}
		sources[index].Imports = uniqueSorted(internal)
	}

	packages := make([]goPackage, 0, len(packageByID))
	for _, pkg := range packageByID {
		pkg.Files = uniqueSorted(pkg.Files)
		packages = append(packages, *pkg)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].ID < packages[j].ID })
	sort.Slice(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
	return sources, packages, nil
}

func receiverNames(fileSet *token.FileSet, function *ast.FuncDecl) (string, string, error) {
	if function.Recv == nil {
		return "", "", nil
	}
	if len(function.Recv.List) != 1 {
		return "", "", fmt.Errorf("method must have exactly one receiver field")
	}
	receiverType := function.Recv.List[0].Type
	var rendered bytes.Buffer
	if err := format.Node(&rendered, fileSet, receiverType); err != nil {
		return "", "", fmt.Errorf("format receiver: %w", err)
	}
	base, err := baseReceiverName(receiverType)
	if err != nil {
		return "", "", err
	}
	return rendered.String(), base, nil
}

func baseReceiverName(expression ast.Expr) (string, error) {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name, nil
	case *ast.StarExpr:
		return baseReceiverName(value.X)
	case *ast.IndexExpr:
		return baseReceiverName(value.X)
	case *ast.IndexListExpr:
		return baseReceiverName(value.X)
	case *ast.ParenExpr:
		return baseReceiverName(value.X)
	default:
		return "", fmt.Errorf("unsupported receiver type %T", expression)
	}
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
