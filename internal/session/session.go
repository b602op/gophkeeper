// Package session управляет сессией пользователя в CLI-клиенте: JWT и
// мастер-ключом шифрования.
//
// Токен сохраняется на диск (права 600), чтобы не вводить пароль при каждом
// запуске. Мастер-ключ хранится только в памяти процесса и живёт ограниченное
// время: на диск он не попадает никогда.
package session

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/b602op/gophkeeper/internal/clientconfig"
	"github.com/b602op/gophkeeper/internal/domain"
)

// masterKeyTTL — время жизни мастер-ключа в памяти процесса.
const masterKeyTTL = 5 * time.Minute

// Ошибки сессии. Проверять их следует через errors.Is.
var (
	// ErrNoToken возвращается, когда пользователь ещё не выполнил вход.
	ErrNoToken = errors.New("токен отсутствует: выполните вход")
	// ErrNoMasterKey возвращается, когда мастер-ключ не задан или истёк.
	ErrNoMasterKey = errors.New("мастер-ключ не установлен или истёк: введите мастер-пароль")
)

// Session хранит токен и мастер-ключ текущего пользователя.
type Session struct {
	tokenPath string
	now       func() time.Time

	mu        sync.Mutex
	token     string
	masterKey []byte
	salt      []byte
	expiresAt time.Time
}

// New создаёт сессию с путём токена по умолчанию.
func New() (*Session, error) {
	path, err := clientconfig.TokenPath()
	if err != nil {
		return nil, err
	}
	return NewWithTokenPath(path), nil
}

// NewWithTokenPath создаёт сессию с явно заданным путём к файлу токена.
//
// Отдельный конструктор нужен тестам и позволяет использовать произвольное
// расположение токена без изменения глобальных каталогов.
func NewWithTokenPath(path string) *Session {
	return &Session{tokenPath: path, now: time.Now}
}

// SetToken сохраняет токен в памяти и записывает его в файл с правами 600.
func (s *Session) SetToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("пустой токен")
	}
	if err := clientconfig.EnsureDir(filepath.Dir(s.tokenPath)); err != nil {
		return err
	}
	if err := os.WriteFile(s.tokenPath, []byte(token), 0o600); err != nil {
		return fmt.Errorf("запись токена: %w", err)
	}

	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
	return nil
}

// GetToken возвращает токен из памяти, а при отсутствии — из файла.
//
// Если токен не сохранён, возвращает ErrNoToken.
func (s *Session) GetToken() (string, error) {
	s.mu.Lock()
	token := s.token
	s.mu.Unlock()
	if token != "" {
		return token, nil
	}

	raw, err := os.ReadFile(s.tokenPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNoToken
		}
		return "", fmt.Errorf("чтение токена: %w", err)
	}

	token = strings.TrimSpace(string(raw))
	if token == "" {
		return "", ErrNoToken
	}

	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
	return token, nil
}

// SetMasterKey кэширует мастер-ключ в памяти на TTL.
func (s *Session) SetMasterKey(key []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.masterKey = append([]byte(nil), key...)
	s.expiresAt = s.now().Add(masterKeyTTL)
}

// SetSalt кэширует соль KDF, использованную для вывода мастер-ключа.
//
// Соль хранится вместе с ключом и живёт тот же TTL: она нужна, чтобы зашифровать
// новую запись тем же ключом.
func (s *Session) SetSalt(salt []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.salt = append([]byte(nil), salt...)
	s.expiresAt = s.now().Add(masterKeyTTL)
}

// GetSalt возвращает кэшированную соль KDF, если TTL не истёк.
func (s *Session) GetSalt() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.salt) == 0 {
		return nil, ErrNoMasterKey
	}
	if s.now().After(s.expiresAt) {
		clear(s.salt)
		s.salt = nil
		return nil, ErrNoMasterKey
	}
	return append([]byte(nil), s.salt...), nil
}

// UserID извлекает идентификатор пользователя из полезной нагрузки JWT.
//
// Подпись намеренно не проверяется: токен получен от сервера, а его полезная
// нагрузка используется только для выбора локального каталога данных.
func (s *Session) UserID() (string, error) {
	token, err := s.GetToken()
	if err != nil {
		return "", err
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("%w: некорректный формат JWT", domain.ErrInvalidToken)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("%w: декодирование JWT: %v", domain.ErrInvalidToken, err)
	}

	var claims struct {
		Subject string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("%w: разбор JWT: %v", domain.ErrInvalidToken, err)
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("%w: в токене отсутствует субъект", domain.ErrInvalidToken)
	}
	return claims.Subject, nil
}

// GetMasterKey возвращает кэшированный мастер-ключ, если TTL не истёк.
//
// По истечении срока ключ затирается и возвращается ErrNoMasterKey.
func (s *Session) GetMasterKey() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.masterKey) == 0 {
		return nil, ErrNoMasterKey
	}
	if s.now().After(s.expiresAt) {
		clear(s.masterKey)
		s.masterKey = nil
		s.expiresAt = time.Time{}
		return nil, ErrNoMasterKey
	}
	return append([]byte(nil), s.masterKey...), nil
}

// Clear очищает сессию: затирает мастер-ключ в памяти и удаляет файл токена.
func (s *Session) Clear() error {
	s.mu.Lock()
	clear(s.masterKey)
	s.masterKey = nil
	clear(s.salt)
	s.salt = nil
	s.expiresAt = time.Time{}
	s.token = ""
	s.mu.Unlock()

	if err := os.Remove(s.tokenPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("удаление токена: %w", err)
	}
	return nil
}
