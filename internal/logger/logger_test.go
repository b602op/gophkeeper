package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNewWithWriter(t *testing.T) {
	tests := []struct {
		name        string
		level       string
		logAt       slog.Level
		wantContain string
		wantEmpty   bool
	}{
		{"debug пропускает debug", "debug", slog.LevelDebug, "level=DEBUG", false},
		{"info отсекает debug", "info", slog.LevelDebug, "", true},
		{"warn пропускает warn", "warn", slog.LevelWarn, "level=WARN", false},
		{"warning синоним warn", "warning", slog.LevelWarn, "level=WARN", false},
		{"error пропускает error", "error", slog.LevelError, "level=ERROR", false},
		{"неизвестный уровень трактуется как info", "unknown", slog.LevelDebug, "", true},
		{"регистр и пробелы игнорируются", "  INFO  ", slog.LevelInfo, "level=INFO", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := NewWithWriter(&buf, tt.level)
			log.Log(context.Background(), tt.logAt, "сообщение")
			got := buf.String()

			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("ожидался пустой вывод, получено: %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantContain) {
				t.Fatalf("вывод %q не содержит %q", got, tt.wantContain)
			}
			if !strings.Contains(got, "сообщение") {
				t.Fatalf("вывод %q не содержит сообщение", got)
			}
		})
	}
}

func TestNew(t *testing.T) {
	if New("info") == nil {
		t.Fatal("New вернул nil")
	}
}
