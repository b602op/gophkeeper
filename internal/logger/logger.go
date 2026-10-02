// Package logger создаёт структурированный логгер на базе log/slog.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// New создаёт slog.Logger, пишущий в stdout с заданным текстовым уровнем.
func New(level string) *slog.Logger {
	return NewWithWriter(os.Stdout, level)
}

// NewWithWriter создаёт slog.Logger с произвольным приёмником вывода.
//
// Отдельный конструктор нужен для тестов: он позволяет проверить формат и
// фильтрацию по уровню без перехвата stdout.
func NewWithWriter(w io.Writer, level string) *slog.Logger {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(handler)
}

// parseLevel переводит строковый уровень в slog.Level. Неизвестное значение
// трактуется как info, чтобы опечатка в конфиге не отключала логирование.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
