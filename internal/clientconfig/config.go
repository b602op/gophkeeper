package clientconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// defaultServerAddress — адрес сервера по умолчанию.
const defaultServerAddress = "http://localhost:8080"

// ErrInvalidConfig сигнализирует о некорректной конфигурации клиента.
var ErrInvalidConfig = errors.New("некорректная конфигурация клиента")

// Config — конфигурация CLI-клиента.
type Config struct {
	// ServerAddress — базовый адрес сервера GophKeeper.
	ServerAddress string `json:"server_address"`
}

// Default возвращает конфигурацию со значениями по умолчанию.
func Default() *Config {
	return &Config{ServerAddress: defaultServerAddress}
}

// Load загружает конфигурацию из XDG-пути.
//
// Отсутствие файла не является ошибкой: свежая установка работает на значениях
// по умолчанию. Некорректный JSON или невалидный адрес приводят к ошибке,
// оборачивающей ErrInvalidConfig.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return loadFrom(path)
}

// loadFrom читает конфигурацию из конкретного пути. Отдельная функция упрощает
// тестирование без подмены каталогов пользователя.
func loadFrom(path string) (*Config, error) {
	cfg := Default()

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("чтение конфигурации %q: %w", path, err)
	}

	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("%w: разбор файла %q: %v", ErrInvalidConfig, path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save сохраняет конфигурацию в XDG-путь, создавая каталог при необходимости.
func Save(cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return saveTo(path, cfg)
}

// saveTo записывает конфигурацию по конкретному пути.
func saveTo(path string, cfg *Config) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("сериализация конфигурации: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("запись конфигурации %q: %w", path, err)
	}
	return nil
}

// Validate проверяет конфигурацию целиком.
//
// Возвращаемая ошибка оборачивает ErrInvalidConfig, чтобы вызывающий код мог
// проверить её через errors.Is.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: конфигурация не задана", ErrInvalidConfig)
	}

	address := strings.TrimSpace(c.ServerAddress)
	if address == "" {
		return fmt.Errorf("%w: server_address обязателен", ErrInvalidConfig)
	}

	parsed, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("%w: некорректный server_address %q: %v", ErrInvalidConfig, address, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: server_address %q должен начинаться с http:// или https://", ErrInvalidConfig, address)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%w: в server_address %q отсутствует хост", ErrInvalidConfig, address)
	}
	return nil
}
