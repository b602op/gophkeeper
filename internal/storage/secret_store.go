package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/b602op/gophkeeper/internal/domain"
)

// Save сохраняет секрет в локальной базе.
//
// Запись перезаписывается целиком по идентификатору; шифрование выполняется
// вызывающей стороной до вызова этого метода.
func (s *SecretStore) Save(secret *domain.Secret) error {
	if secret == nil {
		return errors.New("секрет не задан")
	}
	if strings.TrimSpace(secret.ID) == "" {
		return errors.New("идентификатор секрета пуст")
	}

	data, err := json.Marshal(secret)
	if err != nil {
		return fmt.Errorf("сериализация секрета %q: %w", secret.ID, err)
	}

	return s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSecrets)
		if b == nil {
			return ErrBucketMissing
		}
		if err := b.Put([]byte(secret.ID), data); err != nil {
			return fmt.Errorf("сохранение секрета %q: %w", secret.ID, err)
		}
		return nil
	})
}

// SaveFromServer применяет серверную запись идемпотентно.
//
// В отличие от Save, который безусловно перезаписывает запись по id, этот метод
// сверяет версии:
//
//   - локальной записи нет — сохранить;
//   - локальная версия меньше серверной — перезаписать;
//   - локальная версия не меньше серверной — пропустить.
//
// Это делает применение изменений при синхронизации идемпотентным: повторный
// приход той же записи (например, если клиент завершился между применением
// данных и сохранением last_sync) не ломает состояние, а более старая серверная
// версия не затирает более новую локальную.
func (s *SecretStore) SaveFromServer(secret *domain.Secret) error {
	if secret == nil {
		return errors.New("секрет не задан")
	}
	if strings.TrimSpace(secret.ID) == "" {
		return errors.New("идентификатор секрета пуст")
	}

	data, err := json.Marshal(secret)
	if err != nil {
		return fmt.Errorf("сериализация секрета %q: %w", secret.ID, err)
	}

	return s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSecrets)
		if b == nil {
			return ErrBucketMissing
		}

		// Сверка версий и запись выполняются в одной транзакции: между чтением
		// локальной версии и записью не может вклиниться другое обновление.
		if raw := b.Get([]byte(secret.ID)); raw != nil {
			var local domain.Secret
			if err := json.Unmarshal(raw, &local); err != nil {
				return fmt.Errorf("разбор секрета %q: %w", secret.ID, err)
			}
			if local.Version >= secret.Version {
				return nil
			}
		}

		if err := b.Put([]byte(secret.ID), data); err != nil {
			return fmt.Errorf("сохранение секрета %q: %w", secret.ID, err)
		}
		return nil
	})
}

// Get возвращает секрет по идентификатору.
//
// Отсутствующая запись даёт domain.ErrSecretNotFound.
func (s *SecretStore) Get(id string) (*domain.Secret, error) {
	if strings.TrimSpace(id) == "" {
		return nil, domain.ErrSecretNotFound
	}

	var found *domain.Secret
	err := s.view(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSecrets)
		if b == nil {
			return ErrBucketMissing
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return domain.ErrSecretNotFound
		}
		var secret domain.Secret
		if err := json.Unmarshal(raw, &secret); err != nil {
			return fmt.Errorf("разбор секрета %q: %w", id, err)
		}
		found = &secret
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// List возвращает все секреты пользователя.
func (s *SecretStore) List() ([]*domain.Secret, error) {
	secrets := make([]*domain.Secret, 0)
	err := s.view(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSecrets)
		if b == nil {
			return ErrBucketMissing
		}
		// ForEach прерывается и возвращает ошибку курсора, что заменяет
		// проверку rows.Err() из SQL-хранилищ.
		return b.ForEach(func(key, value []byte) error {
			var secret domain.Secret
			if err := json.Unmarshal(value, &secret); err != nil {
				return fmt.Errorf("разбор секрета %q: %w", string(key), err)
			}
			secrets = append(secrets, &secret)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return secrets, nil
}

// Search ищет секреты по вхождению подстроки в название и метаинформацию.
//
// Поиск регистронезависимый. Пустой запрос возвращает все записи.
func (s *SecretStore) Search(query string) ([]*domain.Secret, error) {
	seq, iterErr := s.searchIter(query)

	matches := make([]*domain.Secret, 0)
	for secret := range seq {
		matches = append(matches, secret)
	}
	// Ошибка обхода (повреждённая запись, закрытая база) не прерывает iter.Seq
	// значением, поэтому проверяется после обхода.
	if *iterErr != nil {
		return nil, *iterErr
	}
	return matches, nil
}

// errStopIteration — внутренний сигнал досрочной остановки обхода. Он не должен
// покидать searchIter: ForEach завершается ошибкой, а searchIter её отфильтровывает.
var errStopIteration = errors.New("обход прерван")

// searchIter возвращает ленивую последовательность секретов, подходящих под
// запрос, и указатель на ошибку обхода.
//
// Ленивость позволяет не собирать весь бакет в промежуточный срез: записи
// читаются, разбираются и фильтруются по одной, что снижает пиковую память на
// больших базах. Обход выполняется в одной транзакции чтения под мьютексом
// хранилища, поэтому вызывать другие методы SecretStore во время обхода нельзя —
// это приведёт к взаимной блокировке.
//
// iter.Seq не умеет возвращать значение, поэтому ошибки (некорректный JSON,
// закрытая база) и сигнал остановки фиксируются в iterErr; вызывающая сторона
// обязана проверить его после обхода.
func (s *SecretStore) searchIter(query string) (iter.Seq[*domain.Secret], *error) {
	needle := strings.ToLower(strings.TrimSpace(query))

	var iterErr error
	seq := func(yield func(*domain.Secret) bool) {
		err := s.view(func(tx *bolt.Tx) error {
			b := tx.Bucket(bucketSecrets)
			if b == nil {
				return ErrBucketMissing
			}
			return b.ForEach(func(key, value []byte) error {
				var secret domain.Secret
				if err := json.Unmarshal(value, &secret); err != nil {
					return fmt.Errorf("разбор секрета %q: %w", string(key), err)
				}
				if needle != "" &&
					!strings.Contains(strings.ToLower(secret.Name), needle) &&
					!strings.Contains(strings.ToLower(secret.Metadata), needle) {
					return nil
				}
				if !yield(&secret) {
					return errStopIteration
				}
				return nil
			})
		})
		if err != nil && !errors.Is(err, errStopIteration) {
			iterErr = err
		}
	}
	return seq, &iterErr
}

// Delete удаляет секрет по идентификатору.
//
// Отсутствующая запись даёт domain.ErrSecretNotFound.
func (s *SecretStore) Delete(id string) error {
	if strings.TrimSpace(id) == "" {
		return domain.ErrSecretNotFound
	}

	return s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSecrets)
		if b == nil {
			return ErrBucketMissing
		}
		if b.Get([]byte(id)) == nil {
			return domain.ErrSecretNotFound
		}
		if err := b.Delete([]byte(id)); err != nil {
			return fmt.Errorf("удаление секрета %q: %w", id, err)
		}
		return nil
	})
}

// GetLastSync возвращает время последней успешной синхронизации.
//
// Если синхронизация ещё не выполнялась, возвращается нулевое время.
func (s *SecretStore) GetLastSync() (time.Time, error) {
	var lastSync time.Time
	err := s.view(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMeta)
		if b == nil {
			return ErrBucketMissing
		}
		raw := b.Get(keyLastSync)
		if raw == nil {
			return nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, string(raw))
		if err != nil {
			return fmt.Errorf("разбор last_sync: %w", err)
		}
		lastSync = parsed
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return lastSync, nil
}

// SetLastSync сохраняет время последней успешной синхронизации.
func (s *SecretStore) SetLastSync(t time.Time) error {
	return s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMeta)
		if b == nil {
			return ErrBucketMissing
		}
		if err := b.Put(keyLastSync, []byte(t.UTC().Format(time.RFC3339Nano))); err != nil {
			return fmt.Errorf("сохранение last_sync: %w", err)
		}
		return nil
	})
}
