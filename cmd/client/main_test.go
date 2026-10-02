package main

import "testing"

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"version", []string{"version"}, false},
		{"help", []string{"help"}, false},
		{"флаг help", []string{"-h"}, false},
		{"длинный флаг help", []string{"--help"}, false},
		{"без аргументов", nil, false},
		{"неизвестная команда", []string{"unknown"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if tt.wantErr && err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("не ожидалась ошибка: %v", err)
			}
		})
	}
}
