package storage

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"

	"github.com/b602op/gophkeeper/internal/domain"
)

// TestOpen проверяет открытие локальной базы по пользователю с путём по
// умолчанию. Каталоги клиента изолируются через t.Setenv: на Linux/macOS
// используется HOME, на Windows — USERPROFILE и LOCALAPPDATA.
func TestOpen(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)          // Linux/macOS
	t.Setenv("USERPROFILE", tmp)   // Windows
	t.Setenv("XDG_DATA_HOME", tmp) // Linux
	t.Setenv("LOCALAPPDATA", tmp)  // Windows: используется в dataDir в первую очередь
	t.Setenv("APPDATA", tmp)

	store, err := Open("test-user-id")
	require.NoError(t, err)
	require.NotNil(t, store)
	defer func() { _ = store.Close() }()
}

func openTestStore(t *testing.T) *SecretStore {
	t.Helper()

	path := filepath.Join(t.TempDir(), "secrets.db")
	store, err := OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath вернул ошибку: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close вернул ошибку: %v", err)
		}
	})
	return store
}

func testSecret(id string) *domain.Secret {
	return &domain.Secret{
		ID:        id,
		UserID:    "user-1",
		Type:      domain.SecretTypeCredentials,
		Name:      "Почта " + id,
		Metadata:  "личное",
		Data:      []byte{0x01, 0x02, 0x03},
		Version:   1,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}
}

func TestOpenCreatesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "secrets.db")

	store, err := OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath вернул ошибку: %v", err)
	}
	defer func() { _ = store.Close() }()

	if _, err := OpenPath(path); err == nil {
		t.Fatal("ожидалась ошибка при повторном открытии занятой базы")
	}
}

func TestSaveGetRoundTrip(t *testing.T) {
	store := openTestStore(t)

	secret := testSecret("id-1")
	if err := store.Save(secret); err != nil {
		t.Fatalf("Save вернул ошибку: %v", err)
	}

	got, err := store.Get("id-1")
	if err != nil {
		t.Fatalf("Get вернул ошибку: %v", err)
	}
	if got.ID != secret.ID || got.Name != secret.Name || got.Type != secret.Type {
		t.Fatalf("Get вернул %+v, ожидалось %+v", got, secret)
	}
	if !bytes.Equal(got.Data, secret.Data) {
		t.Fatalf("Data = %v, ожидалось %v", got.Data, secret.Data)
	}
}

func TestSaveOverwrites(t *testing.T) {
	store := openTestStore(t)

	secret := testSecret("id-1")
	if err := store.Save(secret); err != nil {
		t.Fatalf("Save вернул ошибку: %v", err)
	}

	secret.Name = "Новое имя"
	secret.Version = 2
	if err := store.Save(secret); err != nil {
		t.Fatalf("повторный Save вернул ошибку: %v", err)
	}

	got, err := store.Get("id-1")
	if err != nil {
		t.Fatalf("Get вернул ошибку: %v", err)
	}
	if got.Name != "Новое имя" || got.Version != 2 {
		t.Fatalf("Get вернул %+v, ожидалась обновлённая запись", got)
	}
}

func TestSaveFromServer(t *testing.T) {
	store := openTestStore(t)

	t.Run("новая запись сохраняется", func(t *testing.T) {
		secret := testSecret("id-1")
		secret.Version = 3
		if err := store.SaveFromServer(secret); err != nil {
			t.Fatalf("SaveFromServer вернул ошибку: %v", err)
		}
		got, err := store.Get("id-1")
		if err != nil {
			t.Fatalf("Get вернул ошибку: %v", err)
		}
		if got.Version != 3 {
			t.Fatalf("Version = %d, ожидалось 3", got.Version)
		}
	})

	t.Run("более новая серверная версия перезаписывает", func(t *testing.T) {
		newer := testSecret("id-1")
		newer.Version = 5
		newer.Name = "Серверная новая"
		if err := store.SaveFromServer(newer); err != nil {
			t.Fatalf("SaveFromServer вернул ошибку: %v", err)
		}
		got, err := store.Get("id-1")
		if err != nil {
			t.Fatalf("Get вернул ошибку: %v", err)
		}
		if got.Version != 5 || got.Name != "Серверная новая" {
			t.Fatalf("Get вернул %+v, ожидалась версия 5", got)
		}
	})

	t.Run("более старая серверная версия не перезаписывает", func(t *testing.T) {
		older := testSecret("id-1")
		older.Version = 2
		older.Name = "Серверная старая"
		if err := store.SaveFromServer(older); err != nil {
			t.Fatalf("SaveFromServer вернул ошибку: %v", err)
		}
		got, err := store.Get("id-1")
		if err != nil {
			t.Fatalf("Get вернул ошибку: %v", err)
		}
		if got.Version != 5 || got.Name != "Серверная новая" {
			t.Fatalf("старая версия затёрла новую: %+v", got)
		}
	})

	t.Run("равная версия не перезаписывает (идемпотентность)", func(t *testing.T) {
		same := testSecret("id-1")
		same.Version = 5
		same.Name = "Другое имя"
		if err := store.SaveFromServer(same); err != nil {
			t.Fatalf("SaveFromServer вернул ошибку: %v", err)
		}
		got, err := store.Get("id-1")
		if err != nil {
			t.Fatalf("Get вернул ошибку: %v", err)
		}
		if got.Name != "Серверная новая" {
			t.Fatalf("равная версия перезаписала запись: %+v", got)
		}
	})

	t.Run("пустой id — ошибка", func(t *testing.T) {
		if err := store.SaveFromServer(testSecret("")); err == nil {
			t.Fatal("ожидалась ошибка для пустого id")
		}
	})

	t.Run("nil — ошибка", func(t *testing.T) {
		if err := store.SaveFromServer(nil); err == nil {
			t.Fatal("ожидалась ошибка для nil")
		}
	})
}

func TestSaveInvalid(t *testing.T) {
	store := openTestStore(t)

	if err := store.Save(nil); err == nil {
		t.Fatal("ожидалась ошибка для nil")
	}
	if err := store.Save(&domain.Secret{}); err == nil {
		t.Fatal("ожидалась ошибка для пустого идентификатора")
	}
}

func TestGetNotFound(t *testing.T) {
	store := openTestStore(t)

	if _, err := store.Get("missing"); !errors.Is(err, domain.ErrSecretNotFound) {
		t.Fatalf("ошибка %v не оборачивает ErrSecretNotFound", err)
	}
	if _, err := store.Get(""); !errors.Is(err, domain.ErrSecretNotFound) {
		t.Fatalf("ошибка %v не оборачивает ErrSecretNotFound", err)
	}
}

func TestList(t *testing.T) {
	store := openTestStore(t)

	empty, err := store.List()
	if err != nil {
		t.Fatalf("List вернул ошибку: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("ожидался пустой список, получено %d", len(empty))
	}

	for _, id := range []string{"id-1", "id-2", "id-3"} {
		if saveErr := store.Save(testSecret(id)); saveErr != nil {
			t.Fatalf("Save(%q) вернул ошибку: %v", id, saveErr)
		}
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List вернул ошибку: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List вернул %d записей, ожидалось 3", len(list))
	}
}

func TestSearch(t *testing.T) {
	store := openTestStore(t)

	first := testSecret("id-1")
	first.Name = "Gmail"
	first.Metadata = "рабочая почта"
	second := testSecret("id-2")
	second.Name = "Банк"
	second.Metadata = "карта"

	for _, secret := range []*domain.Secret{first, second} {
		if err := store.Save(secret); err != nil {
			t.Fatalf("Save вернул ошибку: %v", err)
		}
	}

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"по названию", "gmail", 1},
		{"регистронезависимо", "GMAIL", 1},
		{"по метаданным", "карта", 1},
		{"частичное совпадение", "поч", 1},
		{"нет совпадений", "нет-такого", 0},
		{"пустой запрос", "  ", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.Search(tt.query)
			if err != nil {
				t.Fatalf("Search вернул ошибку: %v", err)
			}
			if len(got) != tt.want {
				t.Fatalf("Search вернул %d записей, ожидалось %d", len(got), tt.want)
			}
		})
	}
}

// TestSearchIterStopsEarly проверяет, что ленивый обход корректно реагирует на
// досрочную остановку: сигнал остановки не просачивается наружу как ошибка.
func TestSearchIterStopsEarly(t *testing.T) {
	store := openTestStore(t)
	for _, id := range []string{"id-1", "id-2", "id-3"} {
		if err := store.Save(testSecret(id)); err != nil {
			t.Fatalf("Save(%q) вернул ошибку: %v", id, err)
		}
	}

	seq, iterErr := store.searchIter("")
	seen := 0
	for range seq {
		seen++
		break
	}
	if *iterErr != nil {
		t.Fatalf("iterErr = %v, ожидалось nil", *iterErr)
	}
	if seen != 1 {
		t.Fatalf("обход остановился после %d записей, ожидалась 1", seen)
	}
}

// TestSearchIterPropagatesError проверяет, что ошибка разбора повреждённой
// записи доходит до вызывающей стороны через указатель iterErr.
func TestSearchIterPropagatesError(t *testing.T) {
	store := openTestStore(t)
	err := store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSecrets).Put([]byte("bad"), []byte("{"))
	})
	if err != nil {
		t.Fatalf("Update вернул ошибку: %v", err)
	}

	seq, iterErr := store.searchIter("")
	for range seq {
	}
	if *iterErr == nil {
		t.Fatal("ожидалась ошибка разбора через iterErr")
	}
}

func TestDelete(t *testing.T) {
	store := openTestStore(t)

	if err := store.Save(testSecret("id-1")); err != nil {
		t.Fatalf("Save вернул ошибку: %v", err)
	}

	if err := store.Delete("id-1"); err != nil {
		t.Fatalf("Delete вернул ошибку: %v", err)
	}
	if _, err := store.Get("id-1"); !errors.Is(err, domain.ErrSecretNotFound) {
		t.Fatalf("ошибка %v не оборачивает ErrSecretNotFound", err)
	}
	if err := store.Delete("id-1"); !errors.Is(err, domain.ErrSecretNotFound) {
		t.Fatalf("ошибка %v не оборачивает ErrSecretNotFound", err)
	}
	if err := store.Delete(""); !errors.Is(err, domain.ErrSecretNotFound) {
		t.Fatalf("ошибка %v не оборачивает ErrSecretNotFound", err)
	}
}

func TestLastSync(t *testing.T) {
	store := openTestStore(t)

	zero, err := store.GetLastSync()
	if err != nil {
		t.Fatalf("GetLastSync вернул ошибку: %v", err)
	}
	if !zero.IsZero() {
		t.Fatalf("начальное last_sync = %v, ожидалось нулевое", zero)
	}

	want := time.Now().UTC().Truncate(time.Second)
	if setErr := store.SetLastSync(want); setErr != nil {
		t.Fatalf("SetLastSync вернул ошибку: %v", setErr)
	}

	got, err := store.GetLastSync()
	if err != nil {
		t.Fatalf("GetLastSync вернул ошибку: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("last_sync = %v, ожидалось %v", got, want)
	}
}

func TestCloseIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.db")
	store, err := OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath вернул ошибку: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("первый Close вернул ошибку: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("повторный Close вернул ошибку: %v", err)
	}
}

func TestOperationsAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.db")
	store, err := OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath вернул ошибку: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close вернул ошибку: %v", err)
	}

	if err := store.Save(testSecret("id-1")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Save: ошибка %v не оборачивает ErrClosed", err)
	}
	if _, err := store.Get("id-1"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Get: ошибка %v не оборачивает ErrClosed", err)
	}
	if _, err := store.List(); !errors.Is(err, ErrClosed) {
		t.Fatalf("List: ошибка %v не оборачивает ErrClosed", err)
	}
	if err := store.Delete("id-1"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Delete: ошибка %v не оборачивает ErrClosed", err)
	}
	if _, err := store.GetLastSync(); !errors.Is(err, ErrClosed) {
		t.Fatalf("GetLastSync: ошибка %v не оборачивает ErrClosed", err)
	}
	if err := store.SetLastSync(time.Now()); !errors.Is(err, ErrClosed) {
		t.Fatalf("SetLastSync: ошибка %v не оборачивает ErrClosed", err)
	}
}

func TestCorruptedRecords(t *testing.T) {
	store := openTestStore(t)

	// Кладём в бакет заведомо некорректный JSON, минуя Save.
	err := store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSecrets).Put([]byte("bad"), []byte("{"))
	})
	if err != nil {
		t.Fatalf("Update вернул ошибку: %v", err)
	}

	if _, err := store.Get("bad"); err == nil {
		t.Fatal("ожидалась ошибка разбора секрета")
	}
	if _, err := store.List(); err == nil {
		t.Fatal("ожидалась ошибка разбора списка")
	}
	if _, err := store.Search("x"); err == nil {
		t.Fatal("ожидалась ошибка разбора при поиске")
	}
}

func TestCorruptedLastSync(t *testing.T) {
	store := openTestStore(t)

	err := store.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put(keyLastSync, []byte("не-дата"))
	})
	if err != nil {
		t.Fatalf("Update вернул ошибку: %v", err)
	}

	if _, err := store.GetLastSync(); err == nil {
		t.Fatal("ожидалась ошибка разбора last_sync")
	}
}
