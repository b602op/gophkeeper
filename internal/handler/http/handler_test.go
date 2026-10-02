package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/b602op/gophkeeper/internal/domain"
)

// stubAuth — управляемый мок AuthService.
type stubAuth struct {
	registerFn   func(ctx context.Context, login, password string) (*domain.User, error)
	loginFn      func(ctx context.Context, login, password string) (string, error)
	parseTokenFn func(token string) (string, error)
}

func (s *stubAuth) Register(ctx context.Context, login, password string) (*domain.User, error) {
	if s.registerFn == nil {
		return &domain.User{ID: "id", Login: login}, nil
	}
	return s.registerFn(ctx, login, password)
}

func (s *stubAuth) Login(ctx context.Context, login, password string) (string, error) {
	if s.loginFn == nil {
		return "token", nil
	}
	return s.loginFn(ctx, login, password)
}

func (s *stubAuth) ParseToken(token string) (string, error) {
	if s.parseTokenFn == nil {
		return "user", nil
	}
	return s.parseTokenFn(token)
}

// newTestHandler создаёт хендлер с логгером в никуда.
func newTestHandler(auth AuthService) *Handler {
	return New(auth, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// doRequest выполняет запрос к роутеру и возвращает ответ.
func doRequest(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandleRegister(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		auth       *stubAuth
		wantStatus int
	}{
		{
			name:       "успех",
			body:       `{"login":"alice","password":"password123"}`,
			auth:       &stubAuth{},
			wantStatus: http.StatusCreated,
		},
		{
			name: "конфликт логина",
			body: `{"login":"alice","password":"password123"}`,
			auth: &stubAuth{registerFn: func(context.Context, string, string) (*domain.User, error) {
				return nil, domain.ErrUserAlreadyExists
			}},
			wantStatus: http.StatusConflict,
		},
		{
			name: "ошибка валидации",
			body: `{"login":"a","password":"password123"}`,
			auth: &stubAuth{registerFn: func(context.Context, string, string) (*domain.User, error) {
				return nil, domain.ErrValidation
			}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "внутренняя ошибка",
			body: `{"login":"alice","password":"password123"}`,
			auth: &stubAuth{registerFn: func(context.Context, string, string) (*domain.User, error) {
				return nil, errors.New("сбой БД")
			}},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "битый JSON",
			body:       `{not-json`,
			auth:       &stubAuth{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "неизвестное поле",
			body:       `{"login":"alice","password":"password123","extra":1}`,
			auth:       &stubAuth{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "пустое тело",
			body:       ``,
			auth:       &stubAuth{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.auth)
			rec := doRequest(t, h.Routes(), http.MethodPost, "/api/v1/register", tt.body)
			require.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandleRegisterResponse(t *testing.T) {
	auth := &stubAuth{registerFn: func(_ context.Context, login, _ string) (*domain.User, error) {
		return &domain.User{ID: "user-1", Login: login}, nil
	}}
	h := newTestHandler(auth)
	rec := doRequest(t, h.Routes(), http.MethodPost, "/api/v1/register", `{"login":"alice","password":"password123"}`)

	var resp registerResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "user-1", resp.ID)
	require.Equal(t, "alice", resp.Login)
}

func TestHandleLogin(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		auth       *stubAuth
		wantStatus int
	}{
		{
			name:       "успех",
			body:       `{"login":"alice","password":"password123"}`,
			auth:       &stubAuth{},
			wantStatus: http.StatusOK,
		},
		{
			name: "неверные данные",
			body: `{"login":"alice","password":"wrong"}`,
			auth: &stubAuth{loginFn: func(context.Context, string, string) (string, error) {
				return "", domain.ErrInvalidCredentials
			}},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "внутренняя ошибка",
			body: `{"login":"alice","password":"password123"}`,
			auth: &stubAuth{loginFn: func(context.Context, string, string) (string, error) {
				return "", errors.New("сбой БД")
			}},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "битый JSON",
			body:       `nope`,
			auth:       &stubAuth{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.auth)
			rec := doRequest(t, h.Routes(), http.MethodPost, "/api/v1/login", tt.body)
			require.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandleLoginResponse(t *testing.T) {
	auth := &stubAuth{loginFn: func(context.Context, string, string) (string, error) {
		return "signed-token", nil
	}}
	h := newTestHandler(auth)
	rec := doRequest(t, h.Routes(), http.MethodPost, "/api/v1/login", `{"login":"alice","password":"password123"}`)

	var resp loginResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "signed-token", resp.Token)
}

func TestHandleHealth(t *testing.T) {
	h := newTestHandler(&stubAuth{})
	rec := doRequest(t, h.Routes(), http.MethodGet, "/health", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "ok")
}

func TestRoutesMethodNotAllowed(t *testing.T) {
	h := newTestHandler(&stubAuth{})
	rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/register", "")
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestStatusForError(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{domain.ErrValidation, http.StatusBadRequest},
		{domain.ErrInvalidCredentials, http.StatusUnauthorized},
		{domain.ErrInvalidToken, http.StatusUnauthorized},
		{domain.ErrUserAlreadyExists, http.StatusConflict},
		{domain.ErrUserNotFound, http.StatusNotFound},
		{domain.ErrSecretNotFound, http.StatusNotFound},
		{domain.ErrInvalidSecretType, http.StatusBadRequest},
		{errors.New("прочее"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			require.Equal(t, tt.want, statusForError(tt.err))
		})
	}
}
