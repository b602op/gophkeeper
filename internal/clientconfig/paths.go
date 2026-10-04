// Package clientconfig определяет пути клиента GophKeeper и работу с его
// конфигурационным файлом.
//
// Пути следуют XDG-соглашению: конфигурация — в каталоге пользовательских
// настроек, данные (токен и локальная база) — в каталоге данных. На Windows
// используются переменные APPDATA и LOCALAPPDATA. Пакет не зависит от других
// пакетов проекта.
package clientconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Имена файлов и каталогов клиента.
const (
	appName        = "gophkeeper"
	configFileName = "config.json"
	tokenFileName  = "token"
	secretsDBName  = "secrets.db"
)

// Переменные окружения и функции ОС вынесены в переменные, чтобы тесты могли
// изолироваться от реальных каталогов пользователя, не трогая глобальное
// состояние процесса.
var (
	userConfigDir = os.UserConfigDir
	userHomeDir   = os.UserHomeDir
	getenv        = os.Getenv
)

// ConfigPath возвращает путь к файлу конфигурации клиента.
func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// TokenPath возвращает путь к файлу с JWT.
func TokenPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, tokenFileName), nil
}

// UserDataDir возвращает каталог данных пользователя.
//
// Разделение по user_id не даёт данным разных пользователей смешиваться на
// одном устройстве.
func UserDataDir(userID string) (string, error) {
	if err := validateUserID(userID); err != nil {
		return "", err
	}
	base, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, userID), nil
}

// SecretsDBPath возвращает путь к локальной базе секретов пользователя.
func SecretsDBPath(userID string) (string, error) {
	dir, err := UserDataDir(userID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, secretsDBName), nil
}

// EnsureDir создаёт каталог вместе с родительскими с правами 700.
//
// Права 700 защищают конфигурацию, токен и локальную базу от чтения другими
// пользователями системы.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("создание каталога %q: %w", dir, err)
	}
	return nil
}

// configDir возвращает каталог конфигурации клиента.
func configDir() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("определение каталога конфигурации: %w", err)
	}
	return filepath.Join(base, appName), nil
}

// dataDir возвращает каталог данных клиента.
func dataDir() (string, error) {
	if runtime.GOOS == "windows" {
		if local := getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, appName), nil
		}
	}
	home, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("определение домашнего каталога: %w", err)
	}
	return filepath.Join(home, ".local", "share", appName), nil
}

// validateUserID запрещает пустые и содержащие разделители пути идентификаторы,
// чтобы значение user_id не могло вывести за пределы каталога данных.
func validateUserID(userID string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("пустой идентификатор пользователя")
	}
	if userID == "." || userID == ".." || strings.ContainsAny(userID, `/\`) {
		return fmt.Errorf("недопустимый идентификатор пользователя %q", userID)
	}
	return nil
}
