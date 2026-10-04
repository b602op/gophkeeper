package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRun_Help проверяет, что вывод справки не считается ошибкой.
func TestRun_Help(t *testing.T) {
	err := run([]string{"--help"})
	assert.NoError(t, err)
}

// TestRun_Version проверяет smoke-запуск команды version без обращения к сети.
func TestRun_Version(t *testing.T) {
	err := run([]string{"version"})
	assert.NoError(t, err)
}

// TestRun_UnknownCommand проверяет, что неизвестная команда возвращает ошибку.
func TestRun_UnknownCommand(t *testing.T) {
	err := run([]string{"unknown-cmd"})
	require.Error(t, err)
}

// TestRun_HelpVariants покрывает дополнительные формы вывода справки и запуск
// без аргументов.
func TestRun_HelpVariants(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"команда help", []string{"help"}},
		{"короткий флаг", []string{"-h"}},
		{"без аргументов", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, run(tt.args))
		})
	}
}
