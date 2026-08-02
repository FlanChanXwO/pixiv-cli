package documentation_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestPublicSDKEnglishGoDoc 保证 sdk、sdk/pixiv、sdk/fanbox 的 package comment
// 与每个导出 declaration 都有非空英文 GoDoc，且 function/method 注释以 identifier
// 开头。语言检查只作用于 declaration doc comment，不扫描测试数据、string literal
// 或 internal 实现注释。
func TestPublicSDKEnglishGoDoc(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	packages := []string{"sdk", "sdk/pixiv", "sdk/fanbox"}
	for _, pkg := range packages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			dir := filepath.Join(repositoryRoot, filepath.FromSlash(pkg))
			files := parsePackageFiles(t, dir)
			packageDoc := ""
			for _, file := range files {
				if file.Doc != nil && strings.TrimSpace(file.Doc.Text()) != "" {
					packageDoc = file.Doc.Text()
					break
				}
			}
			if strings.TrimSpace(packageDoc) == "" {
				t.Errorf("%s: package comment is empty", pkg)
			} else if containsCJK(packageDoc) {
				t.Errorf("%s: package comment contains CJK text", pkg)
			}
			checkDoc := func(name, text string, requirePrefix bool) {
				text = strings.TrimSpace(text)
				if text == "" {
					t.Errorf("%s: exported %s has no GoDoc", pkg, name)
					return
				}
				if containsCJK(text) {
					t.Errorf("%s: exported %s GoDoc contains CJK text", pkg, name)
				}
				if requirePrefix && !strings.HasPrefix(text, name) {
					t.Errorf("%s: exported %s GoDoc must start with the identifier", pkg, name)
				}
			}
			for _, file := range files {
				for _, decl := range file.Decls {
					switch node := decl.(type) {
					case *ast.FuncDecl:
						if node.Name.IsExported() && node.Recv == nil {
							checkDoc(node.Name.Name, node.Doc.Text(), true)
						}
						if node.Recv != nil && node.Name.IsExported() && exportedReceiver(node.Recv) {
							checkDoc(node.Name.Name, node.Doc.Text(), true)
						}
					case *ast.GenDecl:
						if node.Tok == token.TYPE {
							for _, spec := range node.Specs {
								if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name.IsExported() {
									checkDoc(typeSpec.Name.Name, firstDoc(node.Doc.Text(), typeSpec.Doc.Text()), false)
								}
							}
						} else if node.Tok == token.CONST || node.Tok == token.VAR {
							groupDoc := node.Doc.Text()
							for _, spec := range node.Specs {
								if valueSpec, ok := spec.(*ast.ValueSpec); ok {
									for _, name := range valueSpec.Names {
										if name.IsExported() {
											checkDoc(name.Name, firstDoc(groupDoc, valueSpec.Doc.Text()), false)
										}
									}
								}
							}
						}
					}
				}
			}
		})
	}
}

// firstDoc 返回 group doc（const/var/type 块的 doc），为空时回退到 spec doc。
func firstDoc(group, spec string) string {
	if strings.TrimSpace(group) != "" {
		return group
	}
	return spec
}

// exportedReceiver 报告 method receiver 是否指向导出类型。
func exportedReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) != 1 {
		return false
	}
	var name string
	switch expr := recv.List[0].Type.(type) {
	case *ast.Ident:
		name = expr.Name
	case *ast.StarExpr:
		if ident, ok := expr.X.(*ast.Ident); ok {
			name = ident.Name
		}
	}
	return name != "" && ast.IsExported(name)
}

func parsePackageFiles(t *testing.T, dir string) []*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	for _, path := range entries {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatalf("no source files in %s", dir)
	}
	return files
}

func containsCJK(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			return true
		}
	}
	return false
}
