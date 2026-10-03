package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/b602op/gophkeeper/internal/domain"
	"github.com/b602op/gophkeeper/internal/middleware"
)

// listResponse — единый ответ для списка секретов и результата синхронизации.
type listResponse struct {
	Secrets []*domain.Secret `json:"secrets"`
}

// handleCreateSecret обрабатывает POST /api/v1/secrets.
func (h *Handler) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	var secret domain.Secret
	if err := decodeJSON(r, &secret); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "некорректное тело запроса")
		return
	}

	if err := h.secrets.Create(r.Context(), userID, &secret); err != nil {
		h.respondServiceError(w, "создание секрета", err)
		return
	}
	writeJSON(h.log, w, http.StatusCreated, secret)
}

// handleListSecrets обрабатывает GET /api/v1/secrets.
func (h *Handler) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	secrets, err := h.secrets.List(r.Context(), userID)
	if err != nil {
		h.respondServiceError(w, "список секретов", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, listResponse{Secrets: secrets})
}

// handleGetSecret обрабатывает GET /api/v1/secrets/{id}.
func (h *Handler) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	secret, err := h.secrets.Get(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		h.respondServiceError(w, "получение секрета", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, secret)
}

// handleUpdateSecret обрабатывает PUT /api/v1/secrets/{id}.
func (h *Handler) handleUpdateSecret(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	var secret domain.Secret
	if err := decodeJSON(r, &secret); err != nil {
		writeError(h.log, w, http.StatusBadRequest, "некорректное тело запроса")
		return
	}
	// Идентификатор берётся из пути: он однозначен и не зависит от тела запроса.
	secret.ID = r.PathValue("id")

	if err := h.secrets.Update(r.Context(), userID, &secret); err != nil {
		h.respondServiceError(w, "обновление секрета", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, secret)
}

// handleDeleteSecret обрабатывает DELETE /api/v1/secrets/{id}.
func (h *Handler) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	if err := h.secrets.Delete(r.Context(), userID, r.PathValue("id")); err != nil {
		h.respondServiceError(w, "удаление секрета", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSync обрабатывает GET /api/v1/sync.
//
// Параметр since задаётся в формате RFC3339. Без параметра возвращаются все
// неудалённые секреты; с параметром — все изменения после указанного момента,
// включая удалённые записи.
func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(h.log, w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	raw := r.URL.Query().Get("since")
	if raw == "" {
		secrets, err := h.secrets.List(r.Context(), userID)
		if err != nil {
			h.respondServiceError(w, "синхронизация секретов", err)
			return
		}
		writeJSON(h.log, w, http.StatusOK, listResponse{Secrets: secrets})
		return
	}

	since, err := ParseSinceParam(raw)
	if err != nil {
		writeError(h.log, w, http.StatusBadRequest, "некорректный параметр since: ожидается RFC3339")
		return
	}

	secrets, err := h.secrets.Sync(r.Context(), userID, since)
	if err != nil {
		h.respondServiceError(w, "синхронизация секретов", err)
		return
	}
	writeJSON(h.log, w, http.StatusOK, listResponse{Secrets: secrets})
}

// ParseSinceParam разбирает параметр since синхронизации.
//
// Ожидается формат ISO 8601 / RFC3339, например "2026-01-15T10:30:00Z".
// Результат приводится к UTC, чтобы сравнение с метками времени в базе было
// корректным независимо от часового пояса клиента.
func ParseSinceParam(raw string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("некорректный since %q: используйте RFC3339, например 2026-01-15T10:30:00Z: %w", raw, err)
	}
	return parsed.UTC(), nil
}
