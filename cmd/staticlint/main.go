// Command staticlint — мультичекер для статического анализа проекта GophKeeper.
//
// Объединяет:
//   - стандартные анализаторы из golang.org/x/tools/go/analysis/passes;
//   - все анализаторы класса SA из staticcheck (поиск ошибок);
//   - выборочные анализаторы классов ST и QF из staticcheck (стиль и быстрые
//     исправления);
//   - собственный анализатор exitcheck, запрещающий os.Exit в main.
//
// Запуск: staticlint ./...
package main

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/asmdecl"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/cgocall"
	"golang.org/x/tools/go/analysis/passes/composite"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"golang.org/x/tools/go/analysis/passes/sortslice"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/stringintconv"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/testinggoroutine"
	"golang.org/x/tools/go/analysis/passes/tests"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unusedresult"
	"honnef.co/go/tools/staticcheck"

	"github.com/b602op/gophkeeper/internal/analysis/exitcheck"
)

func main() {
	analyzers := standardAnalyzers()
	analyzers = append(analyzers, staticcheckAnalyzers()...)
	analyzers = append(analyzers, exitcheck.Analyzer)

	multichecker.Main(analyzers...)
}

// standardAnalyzers возвращает набор стандартных анализаторов x/tools.
func standardAnalyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		asmdecl.Analyzer,
		assign.Analyzer,
		atomic.Analyzer,
		bools.Analyzer,
		buildtag.Analyzer,
		cgocall.Analyzer,
		composite.Analyzer,
		copylock.Analyzer,
		errorsas.Analyzer,
		httpresponse.Analyzer,
		loopclosure.Analyzer,
		lostcancel.Analyzer,
		nilfunc.Analyzer,
		printf.Analyzer,
		shadow.Analyzer,
		sortslice.Analyzer,
		stdmethods.Analyzer,
		stringintconv.Analyzer,
		structtag.Analyzer,
		testinggoroutine.Analyzer,
		tests.Analyzer,
		unmarshal.Analyzer,
		unreachable.Analyzer,
		unusedresult.Analyzer,
	}
}

// staticcheckAnalyzers возвращает анализаторы staticcheck: все класса SA и
// выборочные ST/QF.
func staticcheckAnalyzers() []*analysis.Analyzer {
	selected := map[string]bool{
		"ST1000": true, // отсутствие комментария к пакету
		"ST1003": true, // корректный нейминг
		"ST1016": true, // единообразие имён получателей
		"ST1020": true, // формат комментария к экспортируемой функции
		"ST1021": true, // формат комментария к экспортируемому типу
		"ST1022": true, // формат комментария к экспортируемой переменной
		"QF1001": true, // применение законов Де Моргана
		"QF1003": true, // замена if/else на switch
	}

	var analyzers []*analysis.Analyzer
	for _, v := range staticcheck.Analyzers {
		// Анализаторы класса SA включаются всегда.
		if len(v.Analyzer.Name) >= 2 && v.Analyzer.Name[:2] == "SA" {
			analyzers = append(analyzers, v.Analyzer)
			continue
		}
		if selected[v.Analyzer.Name] {
			analyzers = append(analyzers, v.Analyzer)
		}
	}
	return analyzers
}
