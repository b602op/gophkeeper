package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/b602op/gophkeeper/internal/config"
)

// fakePool — заглушка пула соединений: тесты не поднимают реальную базу.
type fakePool struct {
	closed   bool
	closeErr error
}

func (f *fakePool) DB() *sql.DB { return nil }

func (f *fakePool) Close() error {
	f.closed = true
	return f.closeErr
}

// testConfig возвращает минимально достаточную конфигурацию для run.
func testConfig() *config.Config {
	return &config.Config{
		RunAddress:      "127.0.0.1:0",
		DatabaseDSN:     "postgres://localhost/db",
		JWTSecret:       "super-secret-key-1234567890",
		TokenTTL:        time.Hour,
		LogLevel:        "error",
		BcryptCost:      4,
		ShutdownTimeout: time.Second,
	}
}

// cancelledNotify эмулирует SIGTERM: контекст уже отменён.
func cancelledNotify(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx, func() {}
}

func TestRunGracefulShutdown(t *testing.T) {
	pool := &fakePool{}
	shutdownCalled := false
	var shutdownHadDeadline bool

	deps := serverDeps{
		loadConfig: func([]string) (*config.Config, error) { return testConfig(), nil },
		notify:     cancelledNotify,
		openDB:     func(context.Context, string) (dbPool, error) { return pool, nil },
		serve:      func(*http.Server) error { return http.ErrServerClosed },
		shutdown: func(ctx context.Context) error {
			shutdownCalled = true
			_, shutdownHadDeadline = ctx.Deadline()
			return nil
		},
	}

	if err := run(deps); err != nil {
		t.Fatalf("run вернул ошибку: %v", err)
	}
	if !shutdownCalled {
		t.Fatal("shutdown не был вызван")
	}
	if !shutdownHadDeadline {
		t.Error("в shutdown передан контекст без таймаута")
	}
	if !pool.closed {
		t.Error("пул соединений не закрыт")
	}
}

func TestRunServerError(t *testing.T) {
	pool := &fakePool{}
	shutdownCalled := false

	deps := serverDeps{
		loadConfig: func([]string) (*config.Config, error) { return testConfig(), nil },
		// Контекст не отменяется: сработать должна ветка ошибки сервера.
		notify: func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return context.WithCancel(ctx)
		},
		openDB: func(context.Context, string) (dbPool, error) { return pool, nil },
		serve:  func(*http.Server) error { return errors.New("сбой сервера") },
		shutdown: func(context.Context) error {
			shutdownCalled = true
			return nil
		},
	}

	err := run(deps)
	if err == nil {
		t.Fatal("ожидалась ошибка работы сервера")
	}
	if !strings.Contains(err.Error(), "сбой сервера") {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if shutdownCalled {
		t.Error("shutdown не должен вызываться при ошибке сервера")
	}
	if !pool.closed {
		t.Error("пул соединений не закрыт")
	}
}

func TestRunShutdownError(t *testing.T) {
	deps := serverDeps{
		loadConfig: func([]string) (*config.Config, error) { return testConfig(), nil },
		notify:     cancelledNotify,
		openDB:     func(context.Context, string) (dbPool, error) { return &fakePool{}, nil },
		serve:      func(*http.Server) error { return http.ErrServerClosed },
		shutdown:   func(context.Context) error { return errors.New("остановка не удалась") },
	}

	err := run(deps)
	if err == nil {
		t.Fatal("ожидалась ошибка остановки")
	}
	if !strings.Contains(err.Error(), "остановка не удалась") {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestRunLoadConfigError(t *testing.T) {
	deps := serverDeps{
		loadConfig: func([]string) (*config.Config, error) { return nil, errors.New("битый конфиг") },
	}

	if err := run(deps); err == nil {
		t.Fatal("ожидалась ошибка загрузки конфигурации")
	}
}

func TestRunOpenDBError(t *testing.T) {
	deps := serverDeps{
		loadConfig: func([]string) (*config.Config, error) { return testConfig(), nil },
		notify:     cancelledNotify,
		openDB:     func(context.Context, string) (dbPool, error) { return nil, errors.New("нет базы") },
	}

	if err := run(deps); err == nil {
		t.Fatal("ожидалась ошибка подключения к базе")
	}
}

// TestRunDefaultServeWithoutTLS покрывает production-ветку serve без внедрения:
// неверный адрес заставляет ListenAndServe вернуть ошибку сразу, без реального
// прослушивания порта.
func TestRunDefaultServeWithoutTLS(t *testing.T) {
	cfg := testConfig()
	cfg.RunAddress = "invalid-address"

	deps := serverDeps{
		loadConfig: func([]string) (*config.Config, error) { return cfg, nil },
		notify: func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return context.WithCancel(ctx)
		},
		openDB: func(context.Context, string) (dbPool, error) { return &fakePool{}, nil },
	}

	if err := run(deps); err == nil {
		t.Fatal("ожидалась ошибка прослушивания")
	}
}

// TestRunDefaultServeWithTLS покрывает production-ветку serve с HTTPS: отсутствие
// файлов сертификата заставляет ListenAndServeTLS вернуть ошибку сразу.
func TestRunDefaultServeWithTLS(t *testing.T) {
	cfg := testConfig()
	cfg.EnableHTTPS = true
	cfg.TLSCertFile = "nonexistent.crt"
	cfg.TLSKeyFile = "nonexistent.key"

	deps := serverDeps{
		loadConfig: func([]string) (*config.Config, error) { return cfg, nil },
		notify: func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return context.WithCancel(ctx)
		},
		openDB: func(context.Context, string) (dbPool, error) { return &fakePool{}, nil },
	}

	if err := run(deps); err == nil {
		t.Fatal("ожидалась ошибка HTTPS-прослушивания")
	}
}

// TestDefaultDeps проверяет, что production-зависимости заданы (serve и
// shutdown подставляются внутри run под конкретный *http.Server).
func TestDefaultDeps(t *testing.T) {
	deps := defaultDeps()
	if deps.loadConfig == nil {
		t.Error("loadConfig не задан")
	}
	if deps.notify == nil {
		t.Error("notify не задан")
	}
	if deps.openDB == nil {
		t.Error("openDB не задан")
	}
}

// TestRunPoolCloseError проверяет, что ошибка закрытия пула логируется, но не
// меняет результат успешной остановки.
func TestRunPoolCloseError(t *testing.T) {
	pool := &fakePool{closeErr: errors.New("ошибка закрытия")}

	deps := serverDeps{
		loadConfig: func([]string) (*config.Config, error) { return testConfig(), nil },
		notify:     cancelledNotify,
		openDB:     func(context.Context, string) (dbPool, error) { return pool, nil },
		serve:      func(*http.Server) error { return http.ErrServerClosed },
		shutdown:   func(context.Context) error { return nil },
	}

	if err := run(deps); err != nil {
		t.Fatalf("run вернул ошибку: %v", err)
	}
	if !pool.closed {
		t.Error("пул соединений не закрыт")
	}
}
