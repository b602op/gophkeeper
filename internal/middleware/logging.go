package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// Logging логирует каждый запрос структурированно: метод, путь, статус, размер
// ответа и длительность обработки.
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			log.Info("http-запрос",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.written),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// responseRecorder запоминает статус и объём ответа для логирования.
type responseRecorder struct {
	http.ResponseWriter
	status  int
	written int
}

// WriteHeader запоминает код статуса перед записью.
func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Write считает записанные байты.
func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.written += n
	return n, err
}
