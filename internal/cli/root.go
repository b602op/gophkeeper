// Package cli реализует команды CLI-клиента GophKeeper на базе cobra.
//
// Команды — тонкий слой: они читают ввод, вызывают HTTP-клиент, локальное
// хранилище и криптографию, а результат печатают в поток вывода. Все зависимости
// собраны в структуре app, что позволяет подменять их в интеграционных тестах
// без запуска реального сервера.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/b602op/gophkeeper/internal/api"
	"github.com/b602op/gophkeeper/internal/clientconfig"
	"github.com/b602op/gophkeeper/internal/crypto"
	"github.com/b602op/gophkeeper/internal/domain"
	"github.com/b602op/gophkeeper/internal/session"
	"github.com/b602op/gophkeeper/internal/storage"
)

// apiClient описывает методы HTTP-клиента, нужные командам.
//
// Интерфейс объявлен на стороне потребителя: команды не зависят от конкретной
// реализации и легко тестируются с подставным клиентом.
type apiClient interface {
	SetToken(token string)
	Register(ctx context.Context, login, password string) (*domain.User, error)
	Login(ctx context.Context, login, password string) (string, error)
	CreateSecret(ctx context.Context, secret *domain.Secret) (*domain.Secret, error)
	GetSecret(ctx context.Context, id string) (*domain.Secret, error)
	ListSecrets(ctx context.Context) ([]*domain.Secret, error)
	UpdateSecret(ctx context.Context, secret *domain.Secret) (*domain.Secret, error)
	DeleteSecret(ctx context.Context, id string) error
	Sync(ctx context.Context, since time.Time) (*api.SyncResult, error)
}

// secretStore описывает локальное хранилище секретов, нужное командам.
type secretStore interface {
	Save(secret *domain.Secret) error
	SaveFromServer(secret *domain.Secret) error
	Get(id string) (*domain.Secret, error)
	List() ([]*domain.Secret, error)
	Search(query string) ([]*domain.Secret, error)
	Delete(id string) error
	GetLastSync() (time.Time, error)
	SetLastSync(t time.Time) error
	Close() error
}

// app хранит зависимости команд и потоки ввода-вывода.
type app struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer

	reader *bufio.Reader

	newSession func() (*session.Session, error)
	newClient  func(baseURL string) apiClient
	openStore  func(userID string) (secretStore, error)
	loadConfig func() (*clientconfig.Config, error)
}

// NewRootCommand создаёт корневую команду клиента.
func NewRootCommand() *cobra.Command {
	return newRootCommand(newApp())
}

// newApp собирает приложение с реальными зависимостями.
func newApp() *app {
	return &app{
		in:         os.Stdin,
		out:        os.Stdout,
		errOut:     os.Stderr,
		newSession: session.New,
		newClient:  func(baseURL string) apiClient { return api.New(baseURL) },
		openStore:  func(userID string) (secretStore, error) { return storage.Open(userID) },
		loadConfig: clientconfig.Load,
	}
}

// newRootCommand создаёт корневую команду с внедрёнными зависимостями.
func newRootCommand(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:           "gophkeeper",
		Short:         "GophKeeper — менеджер паролей",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(a.in)
	root.SetOut(a.out)
	root.SetErr(a.errOut)

	root.AddCommand(
		newRegisterCmd(a),
		newLoginCmd(a),
		newLogoutCmd(a),
		newAddCmd(a),
		newGetCmd(a),
		newListCmd(a),
		newSearchCmd(a),
		newUpdateCmd(a),
		newDeleteCmd(a),
		newSyncCmd(a),
		newVersionCmd(a),
	)
	return root
}

// bufReader возвращает постоянный буферизованный читатель стандартного ввода.
//
// Единый reader важен при последовательном чтении нескольких строк: иначе
// буферизация съедала бы данные следующей строки.
func (a *app) bufReader() *bufio.Reader {
	if a.reader == nil {
		a.reader = bufio.NewReader(a.in)
	}
	return a.reader
}

// print печатает сообщение в поток вывода приложения.
func (a *app) print(format string, args ...any) {
	fmt.Fprintf(a.out, format, args...)
}

// authenticatedClient создаёт HTTP-клиент с токеном текущей сессии.
func (a *app) authenticatedClient() (apiClient, *session.Session, error) {
	s, err := a.newSession()
	if err != nil {
		return nil, nil, err
	}

	token, err := s.GetToken()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := a.loadConfig()
	if err != nil {
		return nil, nil, err
	}

	client := a.newClient(cfg.ServerAddress)
	client.SetToken(token)
	return client, s, nil
}

// storeForSession открывает локальное хранилище пользователя текущей сессии.
func (a *app) storeForSession(s *session.Session) (secretStore, error) {
	userID, err := s.UserID()
	if err != nil {
		return nil, err
	}
	return a.openStore(userID)
}

// ensureMasterKey возвращает мастер-ключ из кэша, при необходимости запрашивая
// мастер-пароль и выводя ключ заново.
func (a *app) ensureMasterKey(ctx context.Context, s *session.Session, store secretStore, client apiClient) ([]byte, error) {
	key, err := s.GetMasterKey()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, session.ErrNoMasterKey) {
		return nil, err
	}

	salt, err := a.resolveSalt(ctx, s, store, client)
	if err != nil {
		return nil, err
	}

	password, err := a.readPassword("Мастер-пароль: ")
	if err != nil {
		return nil, err
	}
	if password == "" {
		return nil, errors.New("мастер-пароль не может быть пустым")
	}

	key = crypto.DeriveKey(password, salt)
	s.SetSalt(salt)
	s.SetMasterKey(key)
	return key, nil
}

// resolveSalt определяет соль KDF пользователя.
//
// Порядок поиска: кэш сессии, локальные секреты, сервер, генерация новой соли.
// Соль хранится вместе с шифротекстом, поэтому её можно восстановить из любой
// записи при входе с другого устройства.
func (a *app) resolveSalt(ctx context.Context, s *session.Session, store secretStore, client apiClient) ([]byte, error) {
	if salt, err := s.GetSalt(); err == nil {
		return salt, nil
	}

	if store != nil {
		if secrets, err := store.List(); err == nil {
			if salt, ok := saltFromSecrets(secrets); ok {
				return salt, nil
			}
		}
	}

	if client != nil {
		if secrets, err := client.ListSecrets(ctx); err == nil {
			if salt, ok := saltFromSecrets(secrets); ok {
				return salt, nil
			}
		}
	}

	return crypto.GenerateSalt()
}

// saltFromSecrets извлекает соль KDF из первой записи, содержащей её.
func saltFromSecrets(secrets []*domain.Secret) ([]byte, bool) {
	for _, secret := range secrets {
		if secret == nil {
			continue
		}
		if salt, _, err := crypto.UnpackPayload(secret.Data); err == nil {
			return salt, true
		}
	}
	return nil, false
}
