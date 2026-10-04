package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/b602op/gophkeeper/internal/api"
	"github.com/b602op/gophkeeper/internal/clientconfig"
	"github.com/b602op/gophkeeper/internal/crypto"
	"github.com/b602op/gophkeeper/internal/domain"
	"github.com/b602op/gophkeeper/internal/session"
)

// fixedNow — детерминированное время для тестов.
var fixedNow = time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

// memStore — простое хранилище в памяти для интеграционных тестов команд.
type memStore struct {
	secrets  map[string]*domain.Secret
	lastSync time.Time
	closed   bool
}

func newMemStore() *memStore {
	return &memStore{secrets: make(map[string]*domain.Secret)}
}

func (m *memStore) Save(secret *domain.Secret) error {
	copied := *secret
	m.secrets[secret.ID] = &copied
	return nil
}

func (m *memStore) Get(id string) (*domain.Secret, error) {
	secret, ok := m.secrets[id]
	if !ok {
		return nil, domain.ErrSecretNotFound
	}
	copied := *secret
	return &copied, nil
}

func (m *memStore) List() ([]*domain.Secret, error) {
	list := make([]*domain.Secret, 0, len(m.secrets))
	for _, secret := range m.secrets {
		copied := *secret
		list = append(list, &copied)
	}
	return list, nil
}

func (m *memStore) Search(query string) ([]*domain.Secret, error) {
	all, err := m.List()
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	var matches []*domain.Secret
	for _, secret := range all {
		if strings.Contains(strings.ToLower(secret.Name), needle) ||
			strings.Contains(strings.ToLower(secret.Metadata), needle) {
			matches = append(matches, secret)
		}
	}
	return matches, nil
}

func (m *memStore) Delete(id string) error {
	if _, ok := m.secrets[id]; !ok {
		return domain.ErrSecretNotFound
	}
	delete(m.secrets, id)
	return nil
}

func (m *memStore) GetLastSync() (time.Time, error) { return m.lastSync, nil }

func (m *memStore) SetLastSync(t time.Time) error {
	m.lastSync = t
	return nil
}

func (m *memStore) Close() error {
	m.closed = true
	return nil
}

// fakeClient — подставной HTTP-клиент с настраиваемыми операциями.
type fakeClient struct {
	token string

	registerFn func(ctx context.Context, login, password string) (*domain.User, error)
	loginFn    func(ctx context.Context, login, password string) (string, error)
	createFn   func(ctx context.Context, secret *domain.Secret) (*domain.Secret, error)
	getFn      func(ctx context.Context, id string) (*domain.Secret, error)
	listFn     func(ctx context.Context) ([]*domain.Secret, error)
	updateFn   func(ctx context.Context, secret *domain.Secret) (*domain.Secret, error)
	deleteFn   func(ctx context.Context, id string) error
	syncFn     func(ctx context.Context, since time.Time) ([]*domain.Secret, error)
}

func (f *fakeClient) SetToken(token string) { f.token = token }

func (f *fakeClient) Register(ctx context.Context, login, password string) (*domain.User, error) {
	if f.registerFn != nil {
		return f.registerFn(ctx, login, password)
	}
	return &domain.User{ID: "user-1", Login: login}, nil
}

func (f *fakeClient) Login(ctx context.Context, login, password string) (string, error) {
	if f.loginFn != nil {
		return f.loginFn(ctx, login, password)
	}
	return "fake-token", nil
}

func (f *fakeClient) CreateSecret(ctx context.Context, secret *domain.Secret) (*domain.Secret, error) {
	if f.createFn != nil {
		return f.createFn(ctx, secret)
	}
	created := *secret
	created.ID = "srv-1"
	created.Version = 1
	return &created, nil
}

func (f *fakeClient) GetSecret(ctx context.Context, id string) (*domain.Secret, error) {
	if f.getFn != nil {
		return f.getFn(ctx, id)
	}
	return nil, domain.ErrSecretNotFound
}

func (f *fakeClient) ListSecrets(ctx context.Context) ([]*domain.Secret, error) {
	if f.listFn != nil {
		return f.listFn(ctx)
	}
	return nil, nil
}

func (f *fakeClient) UpdateSecret(ctx context.Context, secret *domain.Secret) (*domain.Secret, error) {
	if f.updateFn != nil {
		return f.updateFn(ctx, secret)
	}
	updated := *secret
	updated.Version++
	return &updated, nil
}

func (f *fakeClient) DeleteSecret(ctx context.Context, id string) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil
}

func (f *fakeClient) Sync(ctx context.Context, since time.Time) ([]*domain.Secret, error) {
	if f.syncFn != nil {
		return f.syncFn(ctx, since)
	}
	return nil, nil
}

// testEnv объединяет подставные зависимости одного теста.
type testEnv struct {
	app    *app
	out    *bytes.Buffer
	store  *memStore
	client *fakeClient
	sess   *session.Session
}

func newTestEnv(t *testing.T, input string) *testEnv {
	t.Helper()

	out := &bytes.Buffer{}
	store := newMemStore()
	client := &fakeClient{}
	sess := session.NewWithTokenPath(filepath.Join(t.TempDir(), "token"))

	a := &app{
		in:         strings.NewReader(input),
		out:        out,
		errOut:     out,
		newSession: func() (*session.Session, error) { return sess, nil },
		newClient:  func(string) apiClient { return client },
		openStore:  func(string) (secretStore, error) { return store, nil },
		loadConfig: func() (*clientconfig.Config, error) {
			return &clientconfig.Config{ServerAddress: "http://test"}, nil
		},
		now: func() time.Time { return fixedNow },
	}
	return &testEnv{app: a, out: out, store: store, client: client, sess: sess}
}

// authenticate записывает в сессию токен с указанным субъектом.
func (e *testEnv) authenticate(t *testing.T, userID string) {
	t.Helper()

	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + userID + `"}`))
	if err := e.sess.SetToken("header." + payload + ".signature"); err != nil {
		t.Fatalf("SetToken вернул ошибку: %v", err)
	}
}

// execute запускает корневую команду с аргументами.
func (e *testEnv) execute(args ...string) error {
	root := newRootCommand(e.app)
	root.SetArgs(args)
	return root.Execute()
}

// encryptSecret создаёт зашифрованный секрет и возвращает его вместе с ключом
// и солью, чтобы установить их в сессию.
func encryptSecret(t *testing.T, secretType domain.SecretType, name string, payload []byte) (*domain.Secret, []byte, []byte) {
	t.Helper()

	salt, err := crypto.GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt вернул ошибку: %v", err)
	}
	key := crypto.DeriveKey("master-pass", salt)
	encoded, err := crypto.Encrypt(key, payload)
	if err != nil {
		t.Fatalf("Encrypt вернул ошибку: %v", err)
	}
	data, err := crypto.PackPayload(salt, encoded)
	if err != nil {
		t.Fatalf("PackPayload вернул ошибку: %v", err)
	}

	secret := &domain.Secret{
		ID:        "id-1",
		UserID:    "user-1",
		Type:      secretType,
		Name:      name,
		Data:      data,
		Version:   1,
		UpdatedAt: fixedNow,
	}
	return secret, key, salt
}

func TestVersionCommand(t *testing.T) {
	env := newTestEnv(t, "")

	if err := env.execute("version"); err != nil {
		t.Fatalf("version вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "version=") {
		t.Fatalf("вывод version = %q", env.out.String())
	}
}

func TestRegister(t *testing.T) {
	env := newTestEnv(t, "server-pass\n")
	env.client.registerFn = func(_ context.Context, login, password string) (*domain.User, error) {
		if login != "alice" || password != "server-pass" {
			t.Errorf("Register получил %q/%q", login, password)
		}
		return &domain.User{ID: "user-1", Login: login}, nil
	}
	env.client.loginFn = func(_ context.Context, login, password string) (string, error) {
		return "jwt-token", nil
	}

	if err := env.execute("register", "--login", "alice"); err != nil {
		t.Fatalf("register вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "зарегистрирован") {
		t.Fatalf("вывод register = %q", env.out.String())
	}
	if token, err := env.sess.GetToken(); err != nil || token != "jwt-token" {
		t.Fatalf("токен = %q, err = %v", token, err)
	}
}

func TestRegisterMissingLogin(t *testing.T) {
	env := newTestEnv(t, "")

	if err := env.execute("register"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestRegisterErrors(t *testing.T) {
	t.Run("ошибка регистрации", func(t *testing.T) {
		env := newTestEnv(t, "pass\n")
		env.client.registerFn = func(context.Context, string, string) (*domain.User, error) {
			return nil, api.ErrConflict
		}
		if err := env.execute("register", "--login", "alice"); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("ошибка входа после регистрации", func(t *testing.T) {
		env := newTestEnv(t, "pass\n")
		env.client.loginFn = func(context.Context, string, string) (string, error) {
			return "", api.ErrUnauthorized
		}
		if err := env.execute("register", "--login", "alice"); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("пустой пароль", func(t *testing.T) {
		env := newTestEnv(t, "\n")
		if err := env.execute("register", "--login", "alice"); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
}

func TestLogin(t *testing.T) {
	env := newTestEnv(t, "server-pass\nmaster-pass\n")
	env.client.loginFn = func(_ context.Context, login, password string) (string, error) {
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user-1"}`))
		return "header." + payload + ".sig", nil
	}

	if err := env.execute("login", "--login", "alice"); err != nil {
		t.Fatalf("login вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "Вход выполнен") {
		t.Fatalf("вывод login = %q", env.out.String())
	}
	if _, err := env.sess.GetMasterKey(); err != nil {
		t.Fatalf("мастер-ключ не кэширован: %v", err)
	}
	if _, err := env.sess.GetSalt(); err != nil {
		t.Fatalf("соль не кэширована: %v", err)
	}
}

func TestLoginErrors(t *testing.T) {
	t.Run("без логина", func(t *testing.T) {
		env := newTestEnv(t, "")
		if err := env.execute("login"); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("ошибка сервера", func(t *testing.T) {
		env := newTestEnv(t, "pass\n")
		env.client.loginFn = func(context.Context, string, string) (string, error) {
			return "", api.ErrUnauthorized
		}
		if err := env.execute("login", "--login", "alice"); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("пустой мастер-пароль", func(t *testing.T) {
		env := newTestEnv(t, "pass\n\n")
		env.client.loginFn = func(context.Context, string, string) (string, error) {
			payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user-1"}`))
			return "header." + payload + ".sig", nil
		}
		if err := env.execute("login", "--login", "alice"); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
}

func TestLogout(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")

	if err := env.execute("logout"); err != nil {
		t.Fatalf("logout вернул ошибку: %v", err)
	}
	if _, err := env.sess.GetToken(); !errors.Is(err, session.ErrNoToken) {
		t.Fatalf("токен не очищен: %v", err)
	}
	if !strings.Contains(env.out.String(), "Выход выполнен") {
		t.Fatalf("вывод logout = %q", env.out.String())
	}
}

func TestAddCredentials(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	var created *domain.Secret
	env.client.createFn = func(_ context.Context, secret *domain.Secret) (*domain.Secret, error) {
		created = secret
		result := *secret
		result.ID = "srv-1"
		result.Version = 1
		return &result, nil
	}

	err := env.execute("add", "Почта", "--type", "credentials",
		"--username", "alice", "--password", "p@ss", "--metadata", "работа")
	if err != nil {
		t.Fatalf("add вернул ошибку: %v", err)
	}
	if created == nil {
		t.Fatal("секрет не отправлен на сервер")
	}
	if created.Name != "Почта" || created.Type != domain.SecretTypeCredentials || created.Metadata != "работа" {
		t.Fatalf("созданный секрет = %+v", created)
	}

	// Данные должны быть зашифрованы, а не содержать открытый пароль.
	if bytes.Contains(created.Data, []byte("p@ss")) {
		t.Fatal("данные секрета не зашифрованы")
	}
	_, encoded, err := crypto.UnpackPayload(created.Data)
	if err != nil {
		t.Fatalf("UnpackPayload вернул ошибку: %v", err)
	}
	key, _ := env.sess.GetMasterKey()
	plaintext, err := crypto.Decrypt(key, encoded)
	if err != nil {
		t.Fatalf("Decrypt вернул ошибку: %v", err)
	}
	if !strings.Contains(string(plaintext), "alice") {
		t.Fatalf("расшифрованные данные = %q", plaintext)
	}

	if _, err := env.store.Get("srv-1"); err != nil {
		t.Fatalf("секрет не сохранён локально: %v", err)
	}
}

func TestAddInteractiveText(t *testing.T) {
	env := newTestEnv(t, "секретный текст\n")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	if err := env.execute("add", "Заметка", "--type", "text"); err != nil {
		t.Fatalf("add вернул ошибку: %v", err)
	}
	if _, err := env.store.Get("srv-1"); err != nil {
		t.Fatalf("секрет не сохранён локально: %v", err)
	}
}

func TestAddBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(path, []byte{0x01, 0x02, 0x03}, 0o600); err != nil {
		t.Fatalf("WriteFile вернул ошибку: %v", err)
	}

	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	if err := env.execute("add", "Файл", "--type", "binary", "--file", path); err != nil {
		t.Fatalf("add вернул ошибку: %v", err)
	}
}

func TestAddErrors(t *testing.T) {
	t.Run("пустое имя", func(t *testing.T) {
		env := newTestEnv(t, "")
		env.authenticate(t, "user-1")
		if err := env.execute("add", ""); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("неверный тип", func(t *testing.T) {
		env := newTestEnv(t, "")
		env.authenticate(t, "user-1")
		if err := env.execute("add", "Имя", "--type", "unknown"); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("не задан файл", func(t *testing.T) {
		env := newTestEnv(t, "")
		env.authenticate(t, "user-1")
		if err := env.execute("add", "Имя", "--type", "binary", "--file", filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
}

func TestAddWithoutAuth(t *testing.T) {
	env := newTestEnv(t, "")

	if err := env.execute("add", "Имя", "--type", "text", "--text", "x"); !errors.Is(err, session.ErrNoToken) {
		t.Fatalf("ошибка %v не оборачивает ErrNoToken", err)
	}
}

func TestGetLocal(t *testing.T) {
	secret, key, salt := encryptSecret(t, domain.SecretTypeCredentials, "Почта", []byte(`{"username":"alice","password":"secret"}`))
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(key)
	if err := env.store.Save(secret); err != nil {
		t.Fatalf("Save вернул ошибку: %v", err)
	}

	if err := env.execute("get", "id-1"); err != nil {
		t.Fatalf("get вернул ошибку: %v", err)
	}
	out := env.out.String()
	if !strings.Contains(out, "alice") || !strings.Contains(out, "secret") {
		t.Fatalf("вывод get = %q", out)
	}
}

func TestGetFromServer(t *testing.T) {
	secret, key, salt := encryptSecret(t, domain.SecretTypeText, "Заметка", []byte(`{"text":"привет"}`))
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(key)
	env.client.getFn = func(context.Context, string) (*domain.Secret, error) {
		return secret, nil
	}

	if err := env.execute("get", "id-1"); err != nil {
		t.Fatalf("get вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "привет") {
		t.Fatalf("вывод get = %q", env.out.String())
	}
	if _, err := env.store.Get("id-1"); err != nil {
		t.Fatalf("секрет не сохранён локально: %v", err)
	}
}

func TestGetNotFound(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")

	if err := env.execute("get", "missing"); !errors.Is(err, domain.ErrSecretNotFound) {
		t.Fatalf("ошибка %v не оборачивает ErrSecretNotFound", err)
	}
}

func TestListAndSearch(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")

	if err := env.execute("list"); err != nil {
		t.Fatalf("list вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "Секретов нет") {
		t.Fatalf("вывод пустого list = %q", env.out.String())
	}

	_ = env.store.Save(&domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Name: "Почта", UpdatedAt: fixedNow})
	_ = env.store.Save(&domain.Secret{ID: "id-2", Type: domain.SecretTypeText, Name: "Банк", UpdatedAt: fixedNow})

	env.out.Reset()
	if err := env.execute("list"); err != nil {
		t.Fatalf("list вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "Почта") || !strings.Contains(env.out.String(), "Банк") {
		t.Fatalf("вывод list = %q", env.out.String())
	}

	env.out.Reset()
	if err := env.execute("search", "поч"); err != nil {
		t.Fatalf("search вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "Почта") || strings.Contains(env.out.String(), "Банк") {
		t.Fatalf("вывод search = %q", env.out.String())
	}

	env.out.Reset()
	if err := env.execute("search", "нет-такого"); err != nil {
		t.Fatalf("search вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "Ничего не найдено") {
		t.Fatalf("вывод search = %q", env.out.String())
	}
}

func TestUpdate(t *testing.T) {
	secret, key, salt := encryptSecret(t, domain.SecretTypeText, "Заметка", []byte(`{"text":"старый"}`))
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(key)
	_ = env.store.Save(secret)

	env.client.updateFn = func(_ context.Context, s *domain.Secret) (*domain.Secret, error) {
		updated := *s
		updated.Version = 2
		return &updated, nil
	}

	if err := env.execute("update", "id-1", "--type", "text", "--text", "новый"); err != nil {
		t.Fatalf("update вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "обновлён") {
		t.Fatalf("вывод update = %q", env.out.String())
	}
	stored, _ := env.store.Get("id-1")
	if stored.Version != 2 {
		t.Fatalf("версия = %d, ожидалось 2", stored.Version)
	}
}

func TestUpdateConflict(t *testing.T) {
	remote := &domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Name: "Заметка", Version: 5, UpdatedAt: fixedNow}

	setup := func(t *testing.T, choice string) *testEnv {
		secret, key, salt := encryptSecret(t, domain.SecretTypeText, "Заметка", []byte(`{"text":"локальный"}`))
		env := newTestEnv(t, choice+"\n")
		env.authenticate(t, "user-1")
		env.sess.SetSalt(salt)
		env.sess.SetMasterKey(key)
		_ = env.store.Save(secret)

		first := true
		env.client.updateFn = func(_ context.Context, s *domain.Secret) (*domain.Secret, error) {
			if first {
				first = false
				return nil, api.ErrConflict
			}
			updated := *s
			updated.Version = remote.Version + 1
			return &updated, nil
		}
		env.client.getFn = func(context.Context, string) (*domain.Secret, error) {
			return remote, nil
		}
		return env
	}

	t.Run("локальная версия", func(t *testing.T) {
		env := setup(t, "l")
		if err := env.execute("update", "id-1", "--text", "локальный"); err != nil {
			t.Fatalf("update вернул ошибку: %v", err)
		}
	})

	t.Run("серверная версия", func(t *testing.T) {
		env := setup(t, "r")
		if err := env.execute("update", "id-1", "--text", "локальный"); err != nil {
			t.Fatalf("update вернул ошибку: %v", err)
		}
		stored, _ := env.store.Get("id-1")
		if stored.Version != remote.Version {
			t.Fatalf("версия = %d, ожидалось %d", stored.Version, remote.Version)
		}
	})

	t.Run("пропустить", func(t *testing.T) {
		env := setup(t, "s")
		if err := env.execute("update", "id-1", "--text", "локальный"); err != nil {
			t.Fatalf("update вернул ошибку: %v", err)
		}
	})

	t.Run("неизвестный выбор", func(t *testing.T) {
		env := setup(t, "x")
		if err := env.execute("update", "id-1", "--text", "локальный"); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
}

func TestDelete(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	_ = env.store.Save(&domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Name: "Заметка"})

	if err := env.execute("delete", "id-1"); err != nil {
		t.Fatalf("delete вернул ошибку: %v", err)
	}
	if _, err := env.store.Get("id-1"); !errors.Is(err, domain.ErrSecretNotFound) {
		t.Fatalf("секрет не удалён: %v", err)
	}
}

func TestDeleteServerNotFound(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	env.client.deleteFn = func(context.Context, string) error {
		return api.ErrNotFound
	}

	if err := env.execute("delete", "id-1"); err != nil {
		t.Fatalf("delete вернул ошибку: %v", err)
	}
}

func TestSync(t *testing.T) {
	remote := &domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Name: "Серверная", Version: 2, UpdatedAt: fixedNow}
	deleted := &domain.Secret{ID: "id-2", Type: domain.SecretTypeText, Name: "Удалённая", DeletedAt: &fixedNow}

	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	_ = env.store.Save(&domain.Secret{ID: "id-2", Type: domain.SecretTypeText, Name: "Локальная копия"})
	_ = env.store.SetLastSync(fixedNow.Add(-time.Hour))

	env.client.syncFn = func(context.Context, time.Time) ([]*domain.Secret, error) {
		return []*domain.Secret{remote, deleted}, nil
	}

	if err := env.execute("sync"); err != nil {
		t.Fatalf("sync вернул ошибку: %v", err)
	}
	if _, err := env.store.Get("id-1"); err != nil {
		t.Fatalf("серверный секрет не сохранён: %v", err)
	}
	if _, err := env.store.Get("id-2"); !errors.Is(err, domain.ErrSecretNotFound) {
		t.Fatalf("удалённый секрет остался локально: %v", err)
	}
	if env.store.lastSync.IsZero() {
		t.Fatal("last_sync не обновлён")
	}
}

func TestSyncPushesLocalChanges(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	_ = env.store.SetLastSync(fixedNow.Add(-time.Hour))
	// Локальная запись изменена после последней синхронизации.
	_ = env.store.Save(&domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Name: "Локальная", UpdatedAt: fixedNow})

	pushed := false
	env.client.updateFn = func(context.Context, *domain.Secret) (*domain.Secret, error) {
		pushed = true
		return &domain.Secret{ID: "id-1", Version: 2}, nil
	}

	if err := env.execute("sync"); err != nil {
		t.Fatalf("sync вернул ошибку: %v", err)
	}
	if !pushed {
		t.Fatal("локальное изменение не отправлено на сервер")
	}
}

func TestSyncFirstRunDoesNotPush(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	_ = env.store.Save(&domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Name: "Локальная", UpdatedAt: fixedNow})

	pushed := false
	env.client.updateFn = func(context.Context, *domain.Secret) (*domain.Secret, error) {
		pushed = true
		return nil, nil
	}

	if err := env.execute("sync"); err != nil {
		t.Fatalf("sync вернул ошибку: %v", err)
	}
	if pushed {
		t.Fatal("при первой синхронизации не должно быть отправки")
	}
}

func TestUnknownCommand(t *testing.T) {
	env := newTestEnv(t, "")

	if err := env.execute("unknown"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestHelp(t *testing.T) {
	env := newTestEnv(t, "")

	if err := env.execute("--help"); err != nil {
		t.Fatalf("help вернул ошибку: %v", err)
	}
}

func TestNewRootCommand(t *testing.T) {
	root := NewRootCommand()
	if root.Use != "gophkeeper" {
		t.Fatalf("Use = %q", root.Use)
	}

	want := map[string]bool{
		"register": false, "login": false, "logout": false, "add": false,
		"get": false, "list": false, "search": false, "update": false,
		"delete": false, "sync": false, "version": false,
	}
	for _, cmd := range root.Commands() {
		if _, ok := want[cmd.Name()]; ok {
			want[cmd.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("команда %q не зарегистрирована", name)
		}
	}
}

func TestAddCardInteractive(t *testing.T) {
	env := newTestEnv(t, "4111111111111111\nAlice\n12/30\n123\n")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	var created *domain.Secret
	env.client.createFn = func(_ context.Context, secret *domain.Secret) (*domain.Secret, error) {
		created = secret
		result := *secret
		result.ID = "srv-1"
		return &result, nil
	}

	if err := env.execute("add", "Карта", "--type", "card"); err != nil {
		t.Fatalf("add вернул ошибку: %v", err)
	}
	if created == nil || created.Type != domain.SecretTypeCard {
		t.Fatalf("созданный секрет = %+v", created)
	}
}

func TestAddCredentialsInteractive(t *testing.T) {
	env := newTestEnv(t, "alice\np@ss\n")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	if err := env.execute("add", "Почта", "--type", "credentials"); err != nil {
		t.Fatalf("add вернул ошибку: %v", err)
	}
}

func TestAddCardMissingNumber(t *testing.T) {
	env := newTestEnv(t, "\n\n\n\n")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	if err := env.execute("add", "Карта", "--type", "card"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestAddTextEmpty(t *testing.T) {
	env := newTestEnv(t, "\n")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	if err := env.execute("add", "Заметка", "--type", "text"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestAddBinaryEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("WriteFile вернул ошибку: %v", err)
	}

	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	if err := env.execute("add", "Файл", "--type", "binary", "--file", path); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestAddCreateError(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))
	env.client.createFn = func(context.Context, *domain.Secret) (*domain.Secret, error) {
		return nil, api.ErrServer
	}

	if err := env.execute("add", "Имя", "--type", "text", "--text", "x"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestGetPromptsMasterKey(t *testing.T) {
	secret, _, _ := encryptSecret(t, domain.SecretTypeText, "Заметка", []byte(`{"text":"привет"}`))
	env := newTestEnv(t, "master-pass\n")
	env.authenticate(t, "user-1")
	// Ключ в сессии намеренно не установлен: команда должна запросить пароль.
	if err := env.store.Save(secret); err != nil {
		t.Fatalf("Save вернул ошибку: %v", err)
	}

	if err := env.execute("get", "id-1"); err != nil {
		t.Fatalf("get вернул ошибку: %v", err)
	}
	if !strings.Contains(env.out.String(), "привет") {
		t.Fatalf("вывод get = %q", env.out.String())
	}
	if _, err := env.sess.GetMasterKey(); err != nil {
		t.Fatalf("мастер-ключ не кэширован: %v", err)
	}
}

func TestGetFormats(t *testing.T) {
	tests := []struct {
		name      string
		secretTyp domain.SecretType
		payload   []byte
		want      string
	}{
		{"credentials", domain.SecretTypeCredentials, []byte(`{"username":"alice","password":"p"}`), "alice"},
		{"card", domain.SecretTypeCard, []byte(`{"number":"4111","holder":"Alice","expiry":"12/30","cvv":"123"}`), "4111"},
		{"text", domain.SecretTypeText, []byte(`{"text":"заметка"}`), "заметка"},
		{"binary", domain.SecretTypeBinary, []byte{0x01, 0x02}, "AQI="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret, key, salt := encryptSecret(t, tt.secretTyp, tt.name, tt.payload)
			env := newTestEnv(t, "")
			env.authenticate(t, "user-1")
			env.sess.SetSalt(salt)
			env.sess.SetMasterKey(key)
			_ = env.store.Save(secret)

			if err := env.execute("get", "id-1"); err != nil {
				t.Fatalf("get вернул ошибку: %v", err)
			}
			if !strings.Contains(env.out.String(), tt.want) {
				t.Fatalf("вывод get = %q, ожидалась подстрока %q", env.out.String(), tt.want)
			}
		})
	}
}

func TestGetCorruptedData(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))
	_ = env.store.Save(&domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Data: []byte("short")})

	if err := env.execute("get", "id-1"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestLoginRecoversSaltFromServer(t *testing.T) {
	secret, _, salt := encryptSecret(t, domain.SecretTypeText, "Заметка", []byte(`{"text":"x"}`))

	env := newTestEnv(t, "server-pass\nmaster-pass\n")
	env.client.loginFn = func(context.Context, string, string) (string, error) {
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user-1"}`))
		return "header." + payload + ".sig", nil
	}
	env.client.listFn = func(context.Context) ([]*domain.Secret, error) {
		return []*domain.Secret{secret}, nil
	}

	if err := env.execute("login", "--login", "alice"); err != nil {
		t.Fatalf("login вернул ошибку: %v", err)
	}

	got, err := env.sess.GetSalt()
	if err != nil {
		t.Fatalf("соль не кэширована: %v", err)
	}
	if !bytes.Equal(got, salt) {
		t.Fatal("соль восстановлена неверно")
	}
}

func TestUpdateFromServer(t *testing.T) {
	existing := &domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Name: "Заметка", Version: 1, UpdatedAt: fixedNow}

	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))
	env.client.getFn = func(context.Context, string) (*domain.Secret, error) {
		return existing, nil
	}
	env.client.updateFn = func(_ context.Context, s *domain.Secret) (*domain.Secret, error) {
		updated := *s
		updated.Version = 2
		return &updated, nil
	}

	if err := env.execute("update", "id-1", "--type", "text", "--text", "новый"); err != nil {
		t.Fatalf("update вернул ошибку: %v", err)
	}
	if _, err := env.store.Get("id-1"); err != nil {
		t.Fatalf("секрет не сохранён: %v", err)
	}
}

func TestDeleteServerError(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	env.client.deleteFn = func(context.Context, string) error {
		return api.ErrServer
	}

	if err := env.execute("delete", "id-1"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestSyncPushConflict(t *testing.T) {
	env := newTestEnv(t, "r\n")
	env.authenticate(t, "user-1")
	_ = env.store.SetLastSync(fixedNow.Add(-time.Hour))
	_ = env.store.Save(&domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Name: "Локальная", Version: 1, UpdatedAt: fixedNow})

	remote := &domain.Secret{ID: "id-1", Type: domain.SecretTypeText, Name: "Серверная", Version: 5, UpdatedAt: fixedNow}
	first := true
	env.client.updateFn = func(_ context.Context, s *domain.Secret) (*domain.Secret, error) {
		if first {
			first = false
			return nil, api.ErrConflict
		}
		return s, nil
	}
	env.client.getFn = func(context.Context, string) (*domain.Secret, error) {
		return remote, nil
	}

	if err := env.execute("sync"); err != nil {
		t.Fatalf("sync вернул ошибку: %v", err)
	}
	stored, err := env.store.Get("id-1")
	if err != nil {
		t.Fatalf("секрет не найден: %v", err)
	}
	if stored.Version != remote.Version {
		t.Fatalf("версия = %d, ожидалось %d", stored.Version, remote.Version)
	}
}

func TestSaltFromSecrets(t *testing.T) {
	if _, ok := saltFromSecrets(nil); ok {
		t.Fatal("nil-список не должен содержать соль")
	}
	if _, ok := saltFromSecrets([]*domain.Secret{nil}); ok {
		t.Fatal("nil-запись не должна содержать соль")
	}
	if _, ok := saltFromSecrets([]*domain.Secret{{Data: []byte("short")}}); ok {
		t.Fatal("короткие данные не должны содержать соль")
	}
}
