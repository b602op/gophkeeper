// Command server запускает HTTP-сервер GophKeeper.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/b602op/gophkeeper/internal/buildinfo"
	"github.com/b602op/gophkeeper/internal/config"
	httpapi "github.com/b602op/gophkeeper/internal/handler/http"
	"github.com/b602op/gophkeeper/internal/idgen"
	"github.com/b602op/gophkeeper/internal/logger"
	"github.com/b602op/gophkeeper/internal/middleware"
	"github.com/b602op/gophkeeper/internal/repository"
	"github.com/b602op/gophkeeper/internal/service"
)

func main() {
	if err := run(); err != nil {
		// main не завершает процесс через os.Exit: логика вынесена в run,
		// здесь только аварийное завершение с понятным сообщением.
		log.Fatalf("сервер остановлен с ошибкой: %v", err)
	}
}

// run собирает зависимости, запускает сервер и корректно освобождает ресурсы.
func run() error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("загрузка конфигурации: %w", err)
	}

	log := logger.New(cfg.LogLevel)
	log.Info("запуск сервера GophKeeper", slog.String("build", buildinfo.String()))

	// NotifyContext отменяет контекст по SIGINT/SIGTERM/SIGQUIT, что запускает
	// единый сценарий graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	pool, err := repository.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("подключение к базе данных: %w", err)
	}
	// Ресурсы освобождаются в обратном порядке относительно создания: сначала
	// останавливается сервер, затем закрывается пул соединений.
	defer func() {
		if closeErr := pool.Close(); closeErr != nil {
			log.Error("закрытие пула соединений", slog.Any("error", closeErr))
		}
	}()

	authService := service.NewAuthService(
		repository.NewUserRepository(pool.DB()),
		cfg.JWTSecret,
		cfg.TokenTTL,
		cfg.BcryptCost,
	)
	secretService := service.NewSecretService(
		repository.NewSecretRepository(pool.DB()),
		idgen.NewUUIDGenerator(),
	)

	handler := httpapi.New(authService, secretService, log)
	router := middleware.Chain(
		handler.Routes(),
		middleware.Recovery(log),
		middleware.Logging(log),
	)

	server := &http.Server{
		Addr:         cfg.RunAddress,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("HTTP-сервер слушает", slog.String("address", cfg.RunAddress), slog.Bool("https", cfg.EnableHTTPS))
		var listenErr error
		if cfg.EnableHTTPS {
			listenErr = server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			listenErr = server.ListenAndServe()
		}
		if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			serverErr <- listenErr
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("работа HTTP-сервера: %w", err)
	case <-ctx.Done():
		log.Info("получен сигнал завершения, останавливаем сервер")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("корректная остановка HTTP-сервера: %w", err)
	}
	log.Info("сервер остановлен")
	return nil
}
