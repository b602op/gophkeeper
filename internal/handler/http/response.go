// Package httpapi содержит HTTP-хендлеры GophKeeper.
//
// Хендлеры — тонкий слой поверх сервисов: они отвечают за разбор запроса,
// выбор кода ответа и сериализацию, но не содержат бизнес-логики. Такую же
// роль для других транспортов (gRPC, CLI) играют их собственные хендлеры,
// которые вызывают те же сервисы.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/b602op/gophkeeper/internal/domain"
)

// errorResponse — единый формат ошибки API.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON сериализует данные в JSON и отправляет их с указанным статусом.
//
// Кодирование выполняется в буфер до записи заголовка, чтобы ошибка кодирования
// не привела к частично отправленному ответу.
func writeJSON(log *slog.Logger, w http.ResponseWriter, status int, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		log.Error("сериализация ответа", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		// Клиент разорвал соединение: ответ доставить уже нельзя.
		log.Debug("запись ответа клиенту", slog.Any("error", err))
	}
}

// writeError отправляет ошибку в едином формате.
func writeError(log *slog.Logger, w http.ResponseWriter, status int, message string) {
	writeJSON(log, w, status, errorResponse{Error: message})
}

// statusForError сопоставляет доменную ошибку с HTTP-статусом.
//
// Централизованное сопоставление гарантирует, что все хендлеры отвечают на
// одинаковые ошибки одинаково.
func statusForError(err error) int {
	switch {
	case errors.Is(err, domain.ErrValidation),
		errors.Is(err, domain.ErrInvalidSecretType),
		errors.Is(err, domain.ErrInvalidSecretData):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrInvalidCredentials),
		errors.Is(err, domain.ErrInvalidToken):
		return http.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrUserAlreadyExists),
		errors.Is(err, domain.ErrSecretAlreadyExists),
		errors.Is(err, domain.ErrSecretVersionMismatch):
		return http.StatusConflict
	case errors.Is(err, domain.ErrUserNotFound),
		errors.Is(err, domain.ErrSecretNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// decodeJSON читает JSON из тела запроса с ограничением размера.
func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}
