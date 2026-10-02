// Package config собирает конфигурацию сервера GophKeeper из четырёх
// источников и валидирует её до старта приложения.
//
// Приоритет источников (от сильного к слабому):
//
//	флаги > переменные окружения > JSON-файл > значения по умолчанию
//
// Конфигурация валидируется целиком после сборки: сервис не должен стартовать
// в заведомо нерабочем состоянии (пустой секрет JWT, битый DSN или адрес).
package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidConfig сигнализирует о некорректной конфигурации.
var ErrInvalidConfig = errors.New("некорректная конфигурация")

// Значения по умолчанию.
const (
	defaultRunAddress      = "localhost:8080"
	defaultLogLevel        = "info"
	defaultTokenTTL        = 24 * time.Hour
	defaultReadTimeout     = 10 * time.Second
	defaultWriteTimeout    = 10 * time.Second
	defaultIdleTimeout     = 60 * time.Second
	defaultShutdownTimeout = 15 * time.Second
	defaultBcryptCost      = 12
	defaultMigrationsDir   = "internal/repository/migrations"

	// minSecretLen — минимальная длина секрета подписи JWT.
	minSecretLen = 16
)

// Имена флагов командной строки.
const (
	flagRunAddress    = "a"
	flagDatabaseDSN   = "d"
	flagJWTSecret     = "jwt-secret"
	flagTokenTTL      = "token-ttl"
	flagLogLevel      = "log-level"
	flagEnableHTTPS   = "https"
	flagTLSCertFile   = "tls-cert"
	flagTLSKeyFile    = "tls-key"
	flagBcryptCost    = "bcrypt-cost"
	flagMigrationsDir = "migrations"
	flagConfigFile    = "c"
)

// Имена переменных окружения.
const (
	envRunAddress    = "GOPHKEEPER_RUN_ADDRESS"
	envDatabaseDSN   = "GOPHKEEPER_DATABASE_DSN"
	envJWTSecret     = "GOPHKEEPER_JWT_SECRET"
	envTokenTTL      = "GOPHKEEPER_TOKEN_TTL"
	envLogLevel      = "GOPHKEEPER_LOG_LEVEL"
	envEnableHTTPS   = "GOPHKEEPER_ENABLE_HTTPS"
	envTLSCertFile   = "GOPHKEEPER_TLS_CERT_FILE"
	envTLSKeyFile    = "GOPHKEEPER_TLS_KEY_FILE"
	envBcryptCost    = "GOPHKEEPER_BCRYPT_COST"
	envMigrationsDir = "GOPHKEEPER_MIGRATIONS_DIR"
	envConfigFile    = "CONFIG_FILE"
)

// Config — итоговая конфигурация сервера.
type Config struct {
	// RunAddress — адрес прослушивания HTTP-сервера в формате host:port.
	RunAddress string
	// DatabaseDSN — строка подключения к PostgreSQL.
	DatabaseDSN string
	// JWTSecret — секрет подписи access-токенов. Обязателен.
	JWTSecret string
	// TokenTTL — время жизни access-токена.
	TokenTTL time.Duration
	// LogLevel — уровень логирования: debug, info, warn, error.
	LogLevel string
	// EnableHTTPS включает TLS для HTTP-сервера.
	EnableHTTPS bool
	// TLSCertFile — путь к файлу сертификата.
	TLSCertFile string
	// TLSKeyFile — путь к файлу приватного ключа.
	TLSKeyFile string
	// ReadTimeout — таймаут чтения HTTP-запроса.
	ReadTimeout time.Duration
	// WriteTimeout — таймаут записи HTTP-ответа.
	WriteTimeout time.Duration
	// IdleTimeout — таймаут простоя keep-alive соединений.
	IdleTimeout time.Duration
	// ShutdownTimeout — максимальное время корректной остановки сервера.
	ShutdownTimeout time.Duration
	// BcryptCost — стоимость хеширования паролей.
	BcryptCost int
	// MigrationsDir — каталог с SQL-миграциями.
	MigrationsDir string
}

// fileConfig — представление JSON-файла конфигурации.
//
// Поля-указатели позволяют отличить явно заданные false/0/пустую строку от
// отсутствующего поля: nil означает «не задано», поэтому значение не перекрывает
// более слабый источник.
type fileConfig struct {
	RunAddress      *string `json:"run_address"`
	DatabaseDSN     *string `json:"database_dsn"`
	JWTSecret       *string `json:"jwt_secret"`
	TokenTTL        *string `json:"token_ttl"`
	LogLevel        *string `json:"log_level"`
	EnableHTTPS     *bool   `json:"enable_https"`
	TLSCertFile     *string `json:"tls_cert_file"`
	TLSKeyFile      *string `json:"tls_key_file"`
	ReadTimeout     *string `json:"read_timeout"`
	WriteTimeout    *string `json:"write_timeout"`
	IdleTimeout     *string `json:"idle_timeout"`
	ShutdownTimeout *string `json:"shutdown_timeout"`
	BcryptCost      *int    `json:"bcrypt_cost"`
	MigrationsDir   *string `json:"migrations_dir"`
}

// flags хранит распарсенные значения флагов и признаки их явного задания.
type flags struct {
	runAddress    string
	databaseDSN   string
	jwtSecret     string
	tokenTTL      time.Duration
	logLevel      string
	enableHTTPS   bool
	tlsCertFile   string
	tlsKeyFile    string
	bcryptCost    int
	migrationsDir string
	configFile    string
	set           map[string]bool
}

// Load собирает конфигурацию из аргументов командной строки, переменных
// окружения и JSON-файла, после чего валидирует результат.
//
// Путь к JSON-файлу задаётся флагом -c или переменной CONFIG_FILE; без них файл
// не читается.
func Load(args []string) (*Config, error) {
	return load(args, os.Getenv)
}

// load — тестируемая реализация Load с внедряемым доступом к переменным
// окружения. Функция getenv позволяет изолировать тесты от глобального state.
func load(args []string, getenv func(string) string) (*Config, error) {
	fs := flag.NewFlagSet("gophkeeper", flag.ContinueOnError)
	f, err := parseFlags(fs, args)
	if err != nil {
		return nil, err
	}

	cfg := defaults()

	configPath := getenv(envConfigFile)
	if f.set[flagConfigFile] {
		configPath = f.configFile
	}
	if configPath != "" {
		if err := applyFile(cfg, configPath); err != nil {
			return nil, err
		}
	}

	if err := applyEnv(cfg, getenv); err != nil {
		return nil, err
	}

	applyFlags(cfg, f)

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// parseFlags объявляет флаги в переданном FlagSet и разбирает args.
//
// Изолированный FlagSet вместо глобального flag.CommandLine нужен, чтобы
// вызывать функцию в тестах без побочных эффектов.
func parseFlags(fs *flag.FlagSet, args []string) (*flags, error) {
	f := &flags{set: make(map[string]bool)}

	fs.StringVar(&f.runAddress, flagRunAddress, defaultRunAddress, "адрес прослушивания HTTP-сервера")
	fs.StringVar(&f.databaseDSN, flagDatabaseDSN, "", "DSN подключения к PostgreSQL")
	fs.StringVar(&f.jwtSecret, flagJWTSecret, "", "секрет подписи JWT")
	fs.DurationVar(&f.tokenTTL, flagTokenTTL, defaultTokenTTL, "время жизни access-токена")
	fs.StringVar(&f.logLevel, flagLogLevel, defaultLogLevel, "уровень логирования")
	fs.BoolVar(&f.enableHTTPS, flagEnableHTTPS, false, "включить HTTPS")
	fs.StringVar(&f.tlsCertFile, flagTLSCertFile, "", "путь к TLS-сертификату")
	fs.StringVar(&f.tlsKeyFile, flagTLSKeyFile, "", "путь к TLS-ключу")
	fs.IntVar(&f.bcryptCost, flagBcryptCost, defaultBcryptCost, "стоимость bcrypt")
	fs.StringVar(&f.migrationsDir, flagMigrationsDir, defaultMigrationsDir, "каталог SQL-миграций")
	fs.StringVar(&f.configFile, flagConfigFile, "", "путь к JSON-файлу конфигурации")

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("разбор флагов: %w", err)
	}

	// Visit перебирает только явно заданные флаги — так мы отличаем «не задан»
	// от «задан значением по умолчанию».
	fs.Visit(func(fl *flag.Flag) { f.set[fl.Name] = true })
	return f, nil
}

// defaults возвращает конфигурацию со значениями по умолчанию.
func defaults() *Config {
	return &Config{
		RunAddress:      defaultRunAddress,
		LogLevel:        defaultLogLevel,
		TokenTTL:        defaultTokenTTL,
		ReadTimeout:     defaultReadTimeout,
		WriteTimeout:    defaultWriteTimeout,
		IdleTimeout:     defaultIdleTimeout,
		ShutdownTimeout: defaultShutdownTimeout,
		BcryptCost:      defaultBcryptCost,
		MigrationsDir:   defaultMigrationsDir,
	}
}

// applyFile накладывает значения из JSON-файла на конфигурацию.
func applyFile(cfg *Config, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w: чтение файла конфигурации %q: %v", ErrInvalidConfig, path, err)
	}

	var fc fileConfig
	if err := json.Unmarshal(raw, &fc); err != nil {
		return fmt.Errorf("%w: разбор файла конфигурации %q: %v", ErrInvalidConfig, path, err)
	}

	setString(&cfg.RunAddress, fc.RunAddress)
	setString(&cfg.DatabaseDSN, fc.DatabaseDSN)
	setString(&cfg.JWTSecret, fc.JWTSecret)
	setString(&cfg.LogLevel, fc.LogLevel)
	setString(&cfg.TLSCertFile, fc.TLSCertFile)
	setString(&cfg.TLSKeyFile, fc.TLSKeyFile)
	setString(&cfg.MigrationsDir, fc.MigrationsDir)
	if fc.EnableHTTPS != nil {
		cfg.EnableHTTPS = *fc.EnableHTTPS
	}
	if fc.BcryptCost != nil {
		cfg.BcryptCost = *fc.BcryptCost
	}

	durations := []struct {
		name  string
		value *string
		dst   *time.Duration
	}{
		{"token_ttl", fc.TokenTTL, &cfg.TokenTTL},
		{"read_timeout", fc.ReadTimeout, &cfg.ReadTimeout},
		{"write_timeout", fc.WriteTimeout, &cfg.WriteTimeout},
		{"idle_timeout", fc.IdleTimeout, &cfg.IdleTimeout},
		{"shutdown_timeout", fc.ShutdownTimeout, &cfg.ShutdownTimeout},
	}
	for _, d := range durations {
		if d.value == nil {
			continue
		}
		parsed, err := time.ParseDuration(*d.value)
		if err != nil {
			return fmt.Errorf("%w: некорректное поле %s=%q в файле %q: %v", ErrInvalidConfig, d.name, *d.value, path, err)
		}
		*d.dst = parsed
	}
	return nil
}

// applyEnv накладывает значения переменных окружения на конфигурацию.
func applyEnv(cfg *Config, getenv func(string) string) error {
	setEnvString(&cfg.RunAddress, getenv, envRunAddress)
	setEnvString(&cfg.DatabaseDSN, getenv, envDatabaseDSN)
	setEnvString(&cfg.JWTSecret, getenv, envJWTSecret)
	setEnvString(&cfg.LogLevel, getenv, envLogLevel)
	setEnvString(&cfg.TLSCertFile, getenv, envTLSCertFile)
	setEnvString(&cfg.TLSKeyFile, getenv, envTLSKeyFile)
	setEnvString(&cfg.MigrationsDir, getenv, envMigrationsDir)

	if v := getenv(envTokenTTL); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%w: некорректная переменная %s=%q: %v", ErrInvalidConfig, envTokenTTL, v, err)
		}
		cfg.TokenTTL = parsed
	}
	if v := getenv(envEnableHTTPS); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("%w: некорректная переменная %s=%q: %v", ErrInvalidConfig, envEnableHTTPS, v, err)
		}
		cfg.EnableHTTPS = parsed
	}
	if v := getenv(envBcryptCost); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%w: некорректная переменная %s=%q: %v", ErrInvalidConfig, envBcryptCost, v, err)
		}
		cfg.BcryptCost = parsed
	}
	return nil
}

// applyFlags накладывает явно заданные флаги на конфигурацию.
func applyFlags(cfg *Config, f *flags) {
	if f.set[flagRunAddress] {
		cfg.RunAddress = f.runAddress
	}
	if f.set[flagDatabaseDSN] {
		cfg.DatabaseDSN = f.databaseDSN
	}
	if f.set[flagJWTSecret] {
		cfg.JWTSecret = f.jwtSecret
	}
	if f.set[flagTokenTTL] {
		cfg.TokenTTL = f.tokenTTL
	}
	if f.set[flagLogLevel] {
		cfg.LogLevel = f.logLevel
	}
	if f.set[flagEnableHTTPS] {
		cfg.EnableHTTPS = f.enableHTTPS
	}
	if f.set[flagTLSCertFile] {
		cfg.TLSCertFile = f.tlsCertFile
	}
	if f.set[flagTLSKeyFile] {
		cfg.TLSKeyFile = f.tlsKeyFile
	}
	if f.set[flagBcryptCost] {
		cfg.BcryptCost = f.bcryptCost
	}
	if f.set[flagMigrationsDir] {
		cfg.MigrationsDir = f.migrationsDir
	}
}

// validate проверяет конфигурацию целиком и возвращает ошибку при первом
// нарушении. Ошибка оборачивает ErrInvalidConfig, чтобы вызывающий код мог
// проверить её через errors.Is.
func (c *Config) validate() error {
	if strings.TrimSpace(c.JWTSecret) == "" {
		return fmt.Errorf("%w: секрет JWT обязателен (флаг -jwt-secret, %s или jwt_secret в файле)", ErrInvalidConfig, envJWTSecret)
	}
	if len(c.JWTSecret) < minSecretLen {
		return fmt.Errorf("%w: длина секрета JWT должна быть не менее %d символов", ErrInvalidConfig, minSecretLen)
	}
	if strings.TrimSpace(c.DatabaseDSN) == "" {
		return fmt.Errorf("%w: DSN базы данных обязателен (флаг -d, %s или database_dsn в файле)", ErrInvalidConfig, envDatabaseDSN)
	}
	if _, err := pgx.ParseConfig(c.DatabaseDSN); err != nil {
		return fmt.Errorf("%w: некорректный DSN %q: %v", ErrInvalidConfig, c.DatabaseDSN, err)
	}
	if err := validateAddress(c.RunAddress); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if c.TokenTTL <= 0 {
		return fmt.Errorf("%w: token_ttl должен быть положительным", ErrInvalidConfig)
	}
	if c.BcryptCost < bcrypt.MinCost || c.BcryptCost > bcrypt.MaxCost {
		return fmt.Errorf("%w: bcrypt_cost должен быть в диапазоне %d..%d", ErrInvalidConfig, bcrypt.MinCost, bcrypt.MaxCost)
	}
	if c.EnableHTTPS && (c.TLSCertFile == "" || c.TLSKeyFile == "") {
		return fmt.Errorf("%w: при включённом HTTPS нужны tls_cert_file и tls_key_file", ErrInvalidConfig)
	}
	return nil
}

// validateAddress проверяет, что адрес задан в формате host:port с корректным
// номером порта. Обёртка нужна, чтобы тестировать собственную логику, а не
// стандартную библиотеку.
func validateAddress(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("адрес %q должен быть в формате host:port: %w", addr, err)
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("порт в адресе %q не является числом: %w", addr, err)
	}
	if p < 1 || p > 65535 {
		return fmt.Errorf("порт %d в адресе %q вне диапазона 1..65535", p, addr)
	}
	return nil
}

// setString присваивает значение по указателю, если он не nil.
func setString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

// setEnvString присваивает значение из окружения, если оно непустое.
func setEnvString(dst *string, getenv func(string) string, key string) {
	if v := getenv(key); v != "" {
		*dst = v
	}
}
