package middleware

import (
	"errors"
	"net/http"
	"strings"
)

// TokenParser проверяет токен и возвращает id пользователя.
type TokenParser interface {
	ParseToken(token string) (string, error)
}

// Auth возвращает middleware, требующий корректный Bearer-токен.
//
// При успешной проверке id пользователя кладётся в контекст и доступен через
// UserIDFromContext. При любой проблеме клиент получает 401 без деталей.
func Auth(parser TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := bearerToken(r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "требуется авторизация")
				return
			}

			userID, err := parser.ParseToken(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "недействительный токен")
				return
			}

			next.ServeHTTP(w, r.WithContext(withUserID(r.Context(), userID)))
		})
	}
}

// bearerToken извлекает токен из заголовка Authorization.
//
// Схема сравнивается без учёта регистра, как требует RFC 6750.
func bearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("отсутствует заголовок Authorization с Bearer-схемой")
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("пустой Bearer-токен")
	}
	return token, nil
}
