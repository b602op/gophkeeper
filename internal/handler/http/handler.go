package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/b602op/gophkeeper/internal/domain"
)

// maxRequestBody — максимальный размер тела запроса.
const maxRequestBody = 1 << 20 // 1 МиБ

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

// Handler собирает HTTP-хендлеры и их зависимости.
type Handler struct {
	auth AuthService
	log  *slog.Logger
}

// New создаёт HTTP-хендлер.
func New(auth AuthService, log *slog.Logger) *Handler {
	return &Handler{auth: auth, log: log}
}

// Routes регистрирует маршруты API и возвращает готовый http.Handler.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/register", h.handleRegister)
	mux.HandleFunc("POST /api/v1/login", h.handleLogin)
	mux.HandleFunc("GET /health", h.handleHealth)
	return mux
}

// handleHealth отвечает на проверку работоспособности сервиса.
func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(h.log, w, http.StatusOK, map[string]string{"status": "ok"})
}
