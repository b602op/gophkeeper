// Package storage реализует локальное хранилище секретов на BoltDB.
//
// Локальная база позволяет работать с записями без постоянных обращений к
// серверу и хранит зашифрованные данные в том виде, в котором они уходят на
// сервер. Каталог базы разделён по пользователям, файл имеет права 600.
package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/b602op/gophkeeper/internal/clientconfig"
)

// Имена бакетов и служебных ключей.
var (
	bucketSecrets = []byte("secrets")
	bucketMeta    = []byte("meta")
	keyLastSync   = []byte("last_sync")
)

// Ошибки хранилища.
var (
	// ErrClosed возвращается при обращении к закрытому хранилищу.
	ErrClosed = errors.New("локальное хранилище закрыто")
	// ErrBucketMissing сигнализирует о повреждённой структуре базы.
	ErrBucketMissing = errors.New("бакет локального хранилища отсутствует")
)

// SecretStore — локальное хранилище секретов пользователя.
type SecretStore struct {
	mu sync.Mutex
	db *bolt.DB
}

// Open открывает локальную базу секретов пользователя.
func Open(userID string) (*SecretStore, error) {
	path, err := clientconfig.SecretsDBPath(userID)
	if err != nil {
		return nil, err
	}
	return OpenPath(path)
}

// OpenPath открывает локальную базу по указанному пути.
//
// Каталог базы создаётся автоматически. Если база уже открыта другим процессом,
// ожидание ограничено одной секундой, после чего возвращается ошибка.
func OpenPath(path string) (*SecretStore, error) {
	if err := clientconfig.EnsureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}

	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("открытие локальной базы %q: %w", path, err)
	}

	store := &SecretStore{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close закрывает базу. Повторный вызов не является ошибкой.
func (s *SecretStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	if err != nil {
		return fmt.Errorf("закрытие локальной базы: %w", err)
	}
	return nil
}

// init создаёт необходимые бакеты в одной транзакции.
func (s *SecretStore) init() error {
	return s.update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketSecrets); err != nil {
			return fmt.Errorf("создание бакета секретов: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists(bucketMeta); err != nil {
			return fmt.Errorf("создание бакета метаданных: %w", err)
		}
		return nil
	})
}

// update выполняет транзакцию записи под блокировкой.
func (s *SecretStore) update(fn func(tx *bolt.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return ErrClosed
	}
	return s.db.Update(fn)
}

// view выполняет транзакцию чтения под блокировкой.
func (s *SecretStore) view(fn func(tx *bolt.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return ErrClosed
	}
	return s.db.View(fn)
}
