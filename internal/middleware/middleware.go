// Package middleware содержит HTTP-middleware GophKeeper: авторизацию по JWT,
// структурированное логирование запросов и перехват паник.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
)

// contextKey — приватный тип ключа контекста, исключающий коллизии с ключами
// других пакетов.
type contextKey int

// userIDKey — ключ, под которым в контексте запроса лежит id пользователя.
const userIDKey contextKey = iota

// UserIDFromContext возвращает id аутентифицированного пользователя.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok
}

// withUserID кладёт id пользователя в контекст запроса.
func withUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// errorResponse — единый формат ошибки в JSON.
type errorResponse struct {
	Error string `json:"error"`
}

// writeError отправляет клиенту ошибку в формате JSON.
func writeError(w http.ResponseWriter, status int, message string) {
	body, err := json.Marshal(errorResponse{Error: message})
	if err != nil {
		// Сериализация простой структуры не может упасть, но на всякий случай
		// отдаём корректный текстовый ответ вместо повреждённого JSON.
		http.Error(w, http.StatusText(status), status)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		// Соединение уже разорвано клиентом — писать больше некуда.
		return
	}
}

// Chain применяет middleware к хендлеру. Первый в списке middleware становится
// самым внешним и обрабатывает запрос первым.
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
