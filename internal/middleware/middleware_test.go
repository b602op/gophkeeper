package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubParser — управляемый мок TokenParser.
type stubParser struct {
	userID string
	err    error
}

func (s stubParser) ParseToken(string) (string, error) {
	return s.userID, s.err
}

// discardLogger возвращает логгер, пишущий в никуда.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// okHandler записывает в ответ id пользователя из контекста.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(id))
	})
}

func TestAuth(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		parser     stubParser
		wantStatus int
		wantUserID string
	}{
		{"успех", "Bearer good-token", stubParser{userID: "user-1"}, http.StatusOK, "user-1"},
		{"схема в другом регистре", "bearer good-token", stubParser{userID: "user-1"}, http.StatusOK, "user-1"},
		{"нет заголовка", "", stubParser{userID: "user-1"}, http.StatusUnauthorized, ""},
		{"неверная схема", "Basic abc", stubParser{userID: "user-1"}, http.StatusUnauthorized, ""},
		{"пустой токен", "Bearer   ", stubParser{userID: "user-1"}, http.StatusUnauthorized, ""},
		{"токен отклонён", "Bearer bad", stubParser{err: errors.New("плохой токен")}, http.StatusUnauthorized, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := Auth(tt.parser)(okHandler())
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantUserID != "" {
				require.Equal(t, tt.wantUserID, rec.Body.String())
			}
		})
	}
}

func TestUserIDFromContextMissing(t *testing.T) {
	_, ok := UserIDFromContext(context.Background())
	require.False(t, ok)
}

func TestLogging(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	handler := Logging(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	out := buf.String()
	require.Contains(t, out, "method=POST")
	require.Contains(t, out, "path=/api/v1/register")
	require.Contains(t, out, "status=201")
	require.Contains(t, out, "bytes=5")
}

func TestRecovery(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	handler := Recovery(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("внезапный сбой")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	require.NotPanics(t, func() { handler.ServeHTTP(rec, req) })

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, buf.String(), "паника")

	var resp errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "внутренняя ошибка сервера", resp.Error)
}

func TestChainOrder(t *testing.T) {
	var order []string
	makeMW := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	final := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		order = append(order, "handler")
	})

	handler := Chain(final, makeMW("first"), makeMW("second"))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, []string{"first", "second", "handler"}, order)
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusUnauthorized, "нет доступа")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	var resp errorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "нет доступа", resp.Error)
}
