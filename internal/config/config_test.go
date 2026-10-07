package config

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testSecret = "super-secret-key-1234567890"
	testDSN    = "postgres://user:pass@localhost:5432/db?sslmode=disable"
)

// envMap создаёт функцию доступа к окружению по карте. Изоляция от os.Getenv
// гарантирует, что тесты не зависят от переменных окружения машины.
func envMap(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

// writeConfig записывает JSON-конфигурацию во временный файл и возвращает путь.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("не удалось записать файл конфигурации: %v", err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load([]string{"-jwt-secret", testSecret, "-d", testDSN}, envMap(nil))
	if err != nil {
		t.Fatalf("load вернул ошибку: %v", err)
	}

	if cfg.RunAddress != defaultRunAddress {
		t.Errorf("RunAddress = %q, ожидалось %q", cfg.RunAddress, defaultRunAddress)
	}
	if cfg.TokenTTL != defaultTokenTTL {
		t.Errorf("TokenTTL = %v, ожидалось %v", cfg.TokenTTL, defaultTokenTTL)
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Errorf("LogLevel = %q, ожидалось %q", cfg.LogLevel, defaultLogLevel)
	}
	if cfg.BcryptCost != defaultBcryptCost {
		t.Errorf("BcryptCost = %d, ожидалось %d", cfg.BcryptCost, defaultBcryptCost)
	}
	if cfg.EnableHTTPS {
		t.Error("EnableHTTPS должен быть false по умолчанию")
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantErr bool
	}{
		{
			name:    "jwt_secret через флаг",
			args:    []string{"-jwt-secret", testSecret, "-d", testDSN},
			env:     map[string]string{},
			wantErr: false,
		},
		{
			name: "jwt_secret через env",
			args: []string{"-d", testDSN},
			env: map[string]string{
				envJWTSecret: testSecret,
			},
			wantErr: false,
		},
		{
			name:    "без jwt_secret — fail early",
			args:    []string{"-d", testDSN},
			env:     map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := load(tt.args, envMap(tt.env))

			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, cfg)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg)
		})
	}
}

func TestLoadPriority(t *testing.T) {
	t.Run("файл перекрывает дефолты", func(t *testing.T) {
		path := writeConfig(t, `{
			"run_address": "0.0.0.0:9000",
			"database_dsn": "`+testDSN+`",
			"jwt_secret": "`+testSecret+`",
			"log_level": "debug",
			"bcrypt_cost": 10,
			"enable_https": true,
			"tls_cert_file": "cert.pem",
			"tls_key_file": "key.pem"
		}`)

		cfg, err := load([]string{"-c", path}, envMap(nil))
		if err != nil {
			t.Fatalf("load вернул ошибку: %v", err)
		}
		if cfg.RunAddress != "0.0.0.0:9000" {
			t.Errorf("RunAddress = %q, ожидалось 0.0.0.0:9000", cfg.RunAddress)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("LogLevel = %q, ожидалось debug", cfg.LogLevel)
		}
		if cfg.BcryptCost != 10 {
			t.Errorf("BcryptCost = %d, ожидалось 10", cfg.BcryptCost)
		}
		if !cfg.EnableHTTPS {
			t.Error("EnableHTTPS должен быть true из файла")
		}
	})

	t.Run("env перекрывает файл", func(t *testing.T) {
		path := writeConfig(t, `{
			"run_address": "0.0.0.0:9000",
			"database_dsn": "`+testDSN+`",
			"jwt_secret": "`+testSecret+`",
			"log_level": "debug"
		}`)

		env := envMap(map[string]string{
			envRunAddress: "127.0.0.1:7000",
			envLogLevel:   "error",
		})
		cfg, err := load([]string{"-c", path}, env)
		if err != nil {
			t.Fatalf("load вернул ошибку: %v", err)
		}
		if cfg.RunAddress != "127.0.0.1:7000" {
			t.Errorf("RunAddress = %q, ожидалось 127.0.0.1:7000", cfg.RunAddress)
		}
		if cfg.LogLevel != "error" {
			t.Errorf("LogLevel = %q, ожидалось error", cfg.LogLevel)
		}
	})

	t.Run("флаги перекрывают env", func(t *testing.T) {
		env := envMap(map[string]string{
			envRunAddress:  "127.0.0.1:7000",
			envLogLevel:    "error",
			envJWTSecret:   testSecret,
			envDatabaseDSN: testDSN,
		})
		cfg, err := load([]string{
			"-a", "localhost:1234",
			"-log-level", "warn",
			"-jwt-secret", testSecret,
			"-d", testDSN,
		}, env)
		if err != nil {
			t.Fatalf("load вернул ошибку: %v", err)
		}
		if cfg.RunAddress != "localhost:1234" {
			t.Errorf("RunAddress = %q, ожидалось localhost:1234", cfg.RunAddress)
		}
		if cfg.LogLevel != "warn" {
			t.Errorf("LogLevel = %q, ожидалось warn", cfg.LogLevel)
		}
	})

	t.Run("CONFIG_FILE из окружения", func(t *testing.T) {
		path := writeConfig(t, `{
			"run_address": "0.0.0.0:9000",
			"database_dsn": "`+testDSN+`",
			"jwt_secret": "`+testSecret+`"
		}`)
		env := envMap(map[string]string{envConfigFile: path})
		cfg, err := load(nil, env)
		if err != nil {
			t.Fatalf("load вернул ошибку: %v", err)
		}
		if cfg.RunAddress != "0.0.0.0:9000" {
			t.Errorf("RunAddress = %q, ожидалось 0.0.0.0:9000", cfg.RunAddress)
		}
	})
}

func TestLoadFilePointerFields(t *testing.T) {
	// Явный false и 0 в файле должны перекрыть ненулевые значения из env.
	path := writeConfig(t, `{
		"database_dsn": "`+testDSN+`",
		"jwt_secret": "`+testSecret+`",
		"enable_https": false,
		"bcrypt_cost": 0
	}`)
	env := envMap(map[string]string{
		envEnableHTTPS: "true",
		envBcryptCost:  "14",
	})

	// bcrypt_cost=0 невалиден, поэтому ожидаем ошибку валидации, но она
	// доказывает, что явный 0 из файла перекрыл env.
	_, err := load([]string{"-c", path}, env)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ожидалась ErrInvalidConfig, получено: %v", err)
	}
}

func TestLoadErrors(t *testing.T) {
	invalidConfig := writeConfig(t, `{not-json`)
	missingFile := filepath.Join(t.TempDir(), "absent.json")

	tests := []struct {
		name string
		args []string
		env  map[string]string
		want error
	}{
		{
			name: "отсутствует секрет JWT",
			args: []string{"-d", testDSN},
			want: ErrInvalidConfig,
		},
		{
			name: "короткий секрет JWT",
			args: []string{"-jwt-secret", "short", "-d", testDSN},
			want: ErrInvalidConfig,
		},
		{
			name: "отсутствует DSN",
			args: []string{"-jwt-secret", testSecret},
			want: ErrInvalidConfig,
		},
		{
			name: "некорректный DSN",
			args: []string{"-jwt-secret", testSecret, "-d", "не-dsn"},
			want: ErrInvalidConfig,
		},
		{
			name: "некорректный адрес",
			args: []string{"-jwt-secret", testSecret, "-d", testDSN, "-a", "localhost"},
			want: ErrInvalidConfig,
		},
		{
			name: "некорректный порт",
			args: []string{"-jwt-secret", testSecret, "-d", testDSN, "-a", "localhost:99999"},
			want: ErrInvalidConfig,
		},
		{
			name: "некорректный bcrypt cost",
			args: []string{"-jwt-secret", testSecret, "-d", testDSN, "-bcrypt-cost", "99"},
			want: ErrInvalidConfig,
		},
		{
			name: "HTTPS без сертификатов",
			args: []string{"-jwt-secret", testSecret, "-d", testDSN, "-https"},
			want: ErrInvalidConfig,
		},
		{
			name: "несуществующий файл",
			args: []string{"-c", missingFile},
			want: ErrInvalidConfig,
		},
		{
			name: "битый JSON",
			args: []string{"-c", invalidConfig},
			want: ErrInvalidConfig,
		},
		{
			name: "некорректный token_ttl в env",
			args: nil,
			env: map[string]string{
				envJWTSecret:   testSecret,
				envDatabaseDSN: testDSN,
				envTokenTTL:    "не-длительность",
			},
			want: ErrInvalidConfig,
		},
		{
			name: "некорректный enable_https в env",
			args: nil,
			env: map[string]string{
				envJWTSecret:   testSecret,
				envDatabaseDSN: testDSN,
				envEnableHTTPS: "не-bool",
			},
			want: ErrInvalidConfig,
		},
		{
			name: "некорректный bcrypt_cost в env",
			args: nil,
			env: map[string]string{
				envJWTSecret:   testSecret,
				envDatabaseDSN: testDSN,
				envBcryptCost:  "не-число",
			},
			want: ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(tt.args, envMap(tt.env))
			if !errors.Is(err, tt.want) {
				t.Fatalf("ожидалась ошибка %v, получено: %v", tt.want, err)
			}
		})
	}
}

func TestLoadInvalidFileDuration(t *testing.T) {
	path := writeConfig(t, `{
		"database_dsn": "`+testDSN+`",
		"jwt_secret": "`+testSecret+`",
		"token_ttl": "abc"
	}`)
	_, err := load([]string{"-c", path}, envMap(nil))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ожидалась ErrInvalidConfig, получено: %v", err)
	}
}

func TestParseFlags(t *testing.T) {
	t.Run("различает явное false от отсутствия", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		f, err := parseFlags(fs, []string{"-https=false"})
		if err != nil {
			t.Fatalf("parseFlags вернул ошибку: %v", err)
		}
		if !f.set[flagEnableHTTPS] {
			t.Error("флаг https должен быть помечен как заданный")
		}
		if f.enableHTTPS {
			t.Error("значение https должно быть false")
		}
		if f.set[flagRunAddress] {
			t.Error("флаг a не задавался и не должен быть в set")
		}
	})

	t.Run("неизвестный флаг", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		if _, err := parseFlags(fs, []string{"-unknown"}); err == nil {
			t.Fatal("ожидалась ошибка для неизвестного флага")
		}
	})

	t.Run("значения по умолчанию не попадают в set", func(t *testing.T) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		f, err := parseFlags(fs, nil)
		if err != nil {
			t.Fatalf("parseFlags вернул ошибку: %v", err)
		}
		if len(f.set) != 0 {
			t.Errorf("set должен быть пуст, получено: %v", f.set)
		}
		if f.tokenTTL != defaultTokenTTL {
			t.Errorf("tokenTTL = %v, ожидалось %v", f.tokenTTL, defaultTokenTTL)
		}
	})
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{"корректный localhost", "localhost:8080", false},
		{"корректный пустой host", ":8080", false},
		{"корректный IP", "127.0.0.1:1", false},
		{"без порта", "localhost", true},
		{"нечисловой порт", "localhost:http", true},
		{"порт вне диапазона", "localhost:70000", true},
		{"нулевой порт", "localhost:0", true},
		{"пустая строка", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAddress(tt.address)
			if tt.wantErr && err == nil {
				t.Fatalf("ожидалась ошибка для адреса %q", tt.address)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("не ожидалась ошибка для адреса %q: %v", tt.address, err)
			}
		})
	}
}

func TestLoadDurationsFromEnv(t *testing.T) {
	env := envMap(map[string]string{
		envJWTSecret:   testSecret,
		envDatabaseDSN: testDSN,
		envTokenTTL:    "30m",
	})
	cfg, err := load(nil, env)
	if err != nil {
		t.Fatalf("load вернул ошибку: %v", err)
	}
	if cfg.TokenTTL != 30*time.Minute {
		t.Errorf("TokenTTL = %v, ожидалось 30m", cfg.TokenTTL)
	}
}

func TestLoadLogLevelTrim(t *testing.T) {
	// Проверяем, что значение из флага сохраняется как есть и не ломает загрузку.
	cfg, err := load([]string{"-jwt-secret", testSecret, "-d", testDSN, "-log-level", " DEBUG "}, envMap(nil))
	if err != nil {
		t.Fatalf("load вернул ошибку: %v", err)
	}
	if strings.TrimSpace(cfg.LogLevel) != "DEBUG" {
		t.Errorf("LogLevel = %q, ожидалось DEBUG", cfg.LogLevel)
	}
}

// TestLoadUsesOSEnvironment проверяет публичную обёртку Load поверх os.Getenv.
func TestLoadUsesOSEnvironment(t *testing.T) {
	t.Setenv(envJWTSecret, testSecret)
	t.Setenv(envDatabaseDSN, testDSN)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load вернул ошибку: %v", err)
	}
	if cfg.JWTSecret != testSecret {
		t.Fatalf("JWTSecret = %q, ожидалось значение из окружения", cfg.JWTSecret)
	}
}

// TestApplyDefaults проверяет, что шаг дефолтов заполняет пустую конфигурацию.
func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	if err := applyDefaults(cfg); err != nil {
		t.Fatalf("applyDefaults вернул ошибку: %v", err)
	}
	if cfg.RunAddress != defaultRunAddress {
		t.Fatalf("RunAddress = %q, ожидалось %q", cfg.RunAddress, defaultRunAddress)
	}
}

// TestApplyFileEmptyPathIsNoop проверяет, что при отсутствии файла шаг ничего
// не делает и не затирает значения предыдущих шагов.
func TestApplyFileEmptyPathIsNoop(t *testing.T) {
	cfg := &Config{RunAddress: "keep:1"}
	if err := applyFile("")(cfg); err != nil {
		t.Fatalf("applyFile(\"\") вернул ошибку: %v", err)
	}
	if cfg.RunAddress != "keep:1" {
		t.Fatalf("applyFile(\"\") изменил конфигурацию: %q", cfg.RunAddress)
	}
}

// TestLoadFileErrorFailsFast проверяет, что ошибка файла прерывает загрузку и не
// маскируется последующими валидными флагами.
func TestLoadFileErrorFailsFast(t *testing.T) {
	invalid := writeConfig(t, `{not-json`)
	_, err := load([]string{"-c", invalid, "-jwt-secret", testSecret, "-d", testDSN}, envMap(nil))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ожидалась ErrInvalidConfig, получено: %v", err)
	}
}
