package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery перехватывает панику в хендлере, логирует её со стеком и отдаёт
// клиенту 500, не раскрывая детали.
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("паника при обработке запроса",
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
					)
					writeError(w, http.StatusInternalServerError, "внутренняя ошибка сервера")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
