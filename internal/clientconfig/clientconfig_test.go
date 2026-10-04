package clientconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// isolate подменяет источники путей на временный каталог и восстанавливает их
// после завершения теста.
func isolate(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	oldConfig, oldHome, oldGetenv := userConfigDir, userHomeDir, getenv

	userConfigDir = func() (string, error) { return dir, nil }
	userHomeDir = func() (string, error) { return dir, nil }
	getenv = func(key string) string {
		if key == "LOCALAPPDATA" {
			return dir
		}
		return ""
	}

	t.Cleanup(func() {
		userConfigDir, userHomeDir, getenv = oldConfig, oldHome, oldGetenv
	})
	return dir
}

// expectedDataDir повторяет логику dataDir для проверки в тестах.
func expectedDataDir(base string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(base, appName)
	}
	return filepath.Join(base, ".local", "share", appName)
}

func TestConfigPath(t *testing.T) {
	base := isolate(t)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath вернул ошибку: %v", err)
	}
	want := filepath.Join(base, appName, configFileName)
	if path != want {
		t.Fatalf("ConfigPath = %q, ожидалось %q", path, want)
	}
}

func TestTokenPath(t *testing.T) {
	base := isolate(t)

	path, err := TokenPath()
	if err != nil {
		t.Fatalf("TokenPath вернул ошибку: %v", err)
	}
	want := filepath.Join(expectedDataDir(base), tokenFileName)
	if path != want {
		t.Fatalf("TokenPath = %q, ожидалось %q", path, want)
	}
}

func TestUserDataDirAndSecretsDBPath(t *testing.T) {
	base := isolate(t)

	dir, err := UserDataDir("user-1")
	if err != nil {
		t.Fatalf("UserDataDir вернул ошибку: %v", err)
	}
	wantDir := filepath.Join(expectedDataDir(base), "user-1")
	if dir != wantDir {
		t.Fatalf("UserDataDir = %q, ожидалось %q", dir, wantDir)
	}

	dbPath, err := SecretsDBPath("user-1")
	if err != nil {
		t.Fatalf("SecretsDBPath вернул ошибку: %v", err)
	}
	wantDB := filepath.Join(wantDir, secretsDBName)
	if dbPath != wantDB {
		t.Fatalf("SecretsDBPath = %q, ожидалось %q", dbPath, wantDB)
	}
}

func TestUserDataDirInvalid(t *testing.T) {
	isolate(t)

	tests := []struct {
		name   string
		userID string
	}{
		{"пустой", ""},
		{"пробелы", "   "},
		{"точка", "."},
		{"две точки", ".."},
		{"слэш", "a/b"},
		{"обратный слэш", `a\b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := UserDataDir(tt.userID); err == nil {
				t.Fatal("ожидалась ошибка")
			}
		})
	}
}

func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")

	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir вернул ошибку: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat вернул ошибку: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("EnsureDir не создал каталог")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("права каталога = %v, ожидалось 0700", info.Mode().Perm())
	}
}

func TestLoadDefaultWhenMissing(t *testing.T) {
	isolate(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load вернул ошибку: %v", err)
	}
	if cfg.ServerAddress != defaultServerAddress {
		t.Fatalf("ServerAddress = %q, ожидалось %q", cfg.ServerAddress, defaultServerAddress)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	isolate(t)

	cfg := &Config{ServerAddress: "https://example.com:8443"}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save вернул ошибку: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load вернул ошибку: %v", err)
	}
	if loaded.ServerAddress != cfg.ServerAddress {
		t.Fatalf("ServerAddress = %q, ожидалось %q", loaded.ServerAddress, cfg.ServerAddress)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	isolate(t)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath вернул ошибку: %v", err)
	}
	if ensureErr := EnsureDir(filepath.Dir(path)); ensureErr != nil {
		t.Fatalf("EnsureDir вернул ошибку: %v", ensureErr)
	}
	if writeErr := os.WriteFile(path, []byte("{"), 0o644); writeErr != nil {
		t.Fatalf("WriteFile вернул ошибку: %v", writeErr)
	}

	_, err = Load()
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ошибка %v не оборачивает ErrInvalidConfig", err)
	}
}

func TestLoadInvalidAddress(t *testing.T) {
	isolate(t)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath вернул ошибку: %v", err)
	}
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		t.Fatalf("EnsureDir вернул ошибку: %v", err)
	}
	raw, _ := json.Marshal(Config{ServerAddress: "ftp://example.com"})
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("WriteFile вернул ошибку: %v", err)
	}

	if _, err := Load(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ошибка %v не оборачивает ErrInvalidConfig", err)
	}
}

func TestLoadReadError(t *testing.T) {
	isolate(t)

	// По пути конфигурации находится каталог: чтение файла должно упасть.
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath вернул ошибку: %v", err)
	}
	if err := EnsureDir(path); err != nil {
		t.Fatalf("EnsureDir вернул ошибку: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("ожидалась ошибка чтения")
	}
}

func TestSaveNil(t *testing.T) {
	isolate(t)

	if err := Save(nil); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ошибка %v не оборачивает ErrInvalidConfig", err)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{"корректный http", &Config{ServerAddress: "http://localhost:8080"}, false},
		{"корректный https", &Config{ServerAddress: "https://example.com"}, false},
		{"nil", nil, true},
		{"пустой", &Config{}, true},
		{"пробелы", &Config{ServerAddress: "   "}, true},
		{"без схемы", &Config{ServerAddress: "localhost:8080"}, true},
		{"неизвестная схема", &Config{ServerAddress: "ftp://example.com"}, true},
		{"без хоста", &Config{ServerAddress: "http://"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("не ожидалась ошибка: %v", err)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("ошибка %v не оборачивает ErrInvalidConfig", err)
			}
		})
	}
}
