package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/b602op/gophkeeper/internal/domain"
)

// credentialsRequest — тело запроса регистрации и входа.
type credentialsRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// registerResponse — ответ на успешную регистрацию.
type registerResponse struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

// loginResponse — ответ на успешный вход.
type loginResponse struct {
	Token string `json:"token"`
}

// handleRegister обрабатывает POST /api/v1/register.
func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "некорректное тело запроса")
		return
	}

	user, err := h.auth.Register(r.Context(), req.Login, req.Password)
	if err != nil {
		h.respondServiceError(w, "регистрация", err)
		return
	}

	// Пароль и хеш в ответ не попадают: логируем и отдаём только id и логин.
	writeJSON(h.log, w, http.StatusCreated, registerResponse{ID: user.ID, Login: user.Login})
}

// handleLogin обрабатывает POST /api/v1/login.
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "некорректное тело запроса")
		return
	}

	token, err := h.auth.Login(r.Context(), req.Login, req.Password)
	if err != nil {
		h.respondServiceError(w, "вход", err)
		return
	}

	writeJSON(h.log, w, http.StatusOK, loginResponse{Token: token})
}

// respondServiceError логирует ошибку сервиса и отправляет клиенту корректный
// статус. Текст внутренней ошибки наружу не выносится.
func (h *Handler) respondServiceError(w http.ResponseWriter, operation string, err error) {
	status := statusForError(err)
	if status == http.StatusInternalServerError {
		h.log.Error("ошибка обработки запроса", slog.String("operation", operation), slog.Any("error", err))
		writeError(h.log, w, status, "внутренняя ошибка сервера")
		return
	}

	h.log.Warn("запрос отклонён",
		slog.String("operation", operation),
		slog.Int("status", status),
		slog.Any("error", err),
	)
	writeError(h.log, w, status, publicMessage(err, status))
}

// publicMessage возвращает безопасное для клиента сообщение по ошибке и
// HTTP-статусу.
//
// Наружу не выносится текст внутренней ошибки: он может содержать детали
// хранилища или конфигурации.
func publicMessage(err error, status int) string {
	switch status {
	case http.StatusBadRequest:
		return "некорректные данные запроса"
	case http.StatusUnauthorized:
		return "неверный логин или пароль"
	case http.StatusForbidden:
		return "доступ запрещён"
	case http.StatusConflict:
		if errors.Is(err, domain.ErrSecretVersionMismatch) {
			return "конфликт версий: запись изменена другим клиентом"
		}
		return "ресурс уже существует"
	case http.StatusNotFound:
		return "ресурс не найден"
	default:
		return "внутренняя ошибка сервера"
	}
}
