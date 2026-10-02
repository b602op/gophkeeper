// Package buildinfo хранит метаданные сборки бинарника.
//
// Значения подставляются на этапе линковки через -ldflags, поэтому их не нужно
// задавать вручную. Пакет используется CLI-командой version и логируется при
// старте сервера.
package buildinfo

import "fmt"

// Version — версия сборки, полученная из git describe.
var Version = "dev"

// BuildDate — дата сборки в формате RFC3339 (UTC).
var BuildDate = "unknown"

// Commit — короткий хеш коммита, из которого собрана версия.
var Commit = "none"

// String возвращает человекочитаемое представление метаданных сборки.
func String() string {
	return fmt.Sprintf("version=%s build_date=%s commit=%s", Version, BuildDate, Commit)
}
