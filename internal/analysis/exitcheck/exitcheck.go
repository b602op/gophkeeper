// Package exitcheck реализует собственный статический анализатор, запрещающий
// вызов os.Exit в функции main пакета main.
//
// Правило вынуждает выносить логику в run() error и завершать процесс только
// через log.Fatalf, что делает поведение предсказуемым и тестируемым.
package exitcheck

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer — анализатор запрета os.Exit в main.
var Analyzer = &analysis.Analyzer{
	Name: "exitcheck",
	Doc:  "запрещает вызов os.Exit в функции main пакета main",
	Run:  run,
}

// run обходит файлы пакета и ищет вызовы os.Exit внутри main.
func run(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	for _, file := range pass.Files {
		// Пропускаем сгенерированные файлы (например, тестовые main из
		// GOCACHE): у них нет расширения .go, и правило к ним неприменимо.
		if !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, ".go") {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "main" || fn.Recv != nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if isOSExit(pass.TypesInfo, call) {
					pass.Reportf(call.Pos(), "использование os.Exit в main запрещено: вынесите логику в run() и завершайте через log.Fatalf")
				}
				return true
			})
		}
	}
	return nil, nil
}

// isOSExit сообщает, что вызов соответствует функции os.Exit.
func isOSExit(info *types.Info, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Exit" {
		return false
	}
	obj, ok := info.Uses[sel.Sel]
	if !ok {
		return false
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	return fn.Pkg() != nil && fn.Pkg().Path() == "os"
}
