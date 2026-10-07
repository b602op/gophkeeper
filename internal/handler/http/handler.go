package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/b602op/gophkeeper/internal/domain"
	"github.com/b602op/gophkeeper/internal/middleware"
)

// maxRequestBody — максимальный размер тела запроса.
//
// С запасом превышает maxSecretSize из сервиса: бинарные данные передаются в
// JSON в base64 и занимают примерно на треть больше исходного объёма.
const maxRequestBody = 2 << 20 // 2 МиБ

// AuthService описывает бизнес-операции аутентификации, необходимые хендлерам.
//
// Интерфейс объявлен на стороне потребителя: хендлер не зависит от конкретной
// реализации сервиса.
type AuthService interface {
	// Register создаёт нового пользователя.
	Register(ctx context.Context, login, password string) (*domain.User, error)
	// Login проверяет учётные данные и возвращает JWT.
	Login(ctx context.Context, login, password string) (string, error)
	// ParseToken проверяет токен и возвращает id пользователя.
	ParseToken(token string) (string, error)
}

// SecretService описывает бизнес-операции над секретами, необходимые хендлерам.
type SecretService interface {
	// Create создаёт секрет пользователя.
	Create(ctx context.Context, userID string, secret *domain.Secret) error
	// Get возвращает секрет пользователя.
	Get(ctx context.Context, userID, id string) (*domain.Secret, error)
	// List возвращает неудалённые секреты пользователя.
	List(ctx context.Context, userID string) ([]*domain.Secret, error)
	// Update обновляет секрет с проверкой версии.
	Update(ctx context.Context, userID string, secret *domain.Secret) error
	// Delete помечает секрет удалённым.
	Delete(ctx context.Context, userID, id string) error
	// Sync возвращает секреты, изменённые после since.
	Sync(ctx context.Context, userID string, since time.Time) ([]*domain.Secret, error)
}

// Handler собирает HTTP-хендлеры и их зависимости.
type Handler struct {
	auth    AuthService
	secrets SecretService
	log     *slog.Logger

	// now внедряется для детерминированных тестов watermark синхронизации.
	now func() time.Time
}

// New создаёт HTTP-хендлер.
func New(auth AuthService, secrets SecretService, log *slog.Logger) *Handler {
	return &Handler{auth: auth, secrets: secrets, log: log, now: time.Now}
}

// Routes регистрирует маршруты API и возвращает готовый http.Handler.
//
// Публичные маршруты (регистрация, вход, health) доступны без токена, остальные
// обёрнуты middleware авторизации.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/register", h.handleRegister)
	mux.HandleFunc("POST /api/v1/login", h.handleLogin)
	mux.HandleFunc("GET /health", h.handleHealth)

	authorized := middleware.Auth(h.auth)
	mux.Handle("POST /api/v1/secrets", authorized(http.HandlerFunc(h.handleCreateSecret)))
	mux.Handle("GET /api/v1/secrets", authorized(http.HandlerFunc(h.handleListSecrets)))
	mux.Handle("GET /api/v1/secrets/{id}", authorized(http.HandlerFunc(h.handleGetSecret)))
	mux.Handle("PUT /api/v1/secrets/{id}", authorized(http.HandlerFunc(h.handleUpdateSecret)))
	mux.Handle("DELETE /api/v1/secrets/{id}", authorized(http.HandlerFunc(h.handleDeleteSecret)))
	mux.Handle("GET /api/v1/sync", authorized(http.HandlerFunc(h.handleSync)))
	return mux
}

// handleHealth отвечает на проверку работоспособности сервиса.
func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(h.log, w, http.StatusOK, map[string]string{"status": "ok"})
}
