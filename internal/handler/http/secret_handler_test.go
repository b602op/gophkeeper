package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/b602op/gophkeeper/internal/domain"
)

// secretBody — корректное тело запроса создания/обновления секрета.
const secretBody = `{"type":"credentials","name":"Почта","metadata":"личное","data":"c2VjcmV0","version":1}`

// testSecret возвращает секрет для ответов моков.
func testSecret() *domain.Secret {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	return &domain.Secret{
		ID:        "secret-1",
		UserID:    "user-1",
		Type:      domain.SecretTypeCredentials,
		Name:      "Почта",
		Metadata:  "личное",
		Data:      []byte("secret"),
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestHandleCreateSecret(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		var gotUserID string
		secrets := &stubSecrets{createFn: func(_ context.Context, userID string, s *domain.Secret) error {
			gotUserID = userID
			s.ID = "secret-1"
			s.Version = 1
			return nil
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodPost, "/api/v1/secrets", secretBody)

		require.Equal(t, http.StatusCreated, rec.Code)
		require.Equal(t, "user-1", gotUserID)

		var resp domain.Secret
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, "secret-1", resp.ID)
		require.Equal(t, domain.SecretTypeCredentials, resp.Type)
	})

	t.Run("валидация", func(t *testing.T) {
		secrets := &stubSecrets{createFn: func(context.Context, string, *domain.Secret) error {
			return domain.ErrInvalidSecretData
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodPost, "/api/v1/secrets", secretBody)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("невалидный тип", func(t *testing.T) {
		secrets := &stubSecrets{createFn: func(context.Context, string, *domain.Secret) error {
			return domain.ErrInvalidSecretType
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodPost, "/api/v1/secrets", secretBody)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("внутренняя ошибка", func(t *testing.T) {
		secrets := &stubSecrets{createFn: func(context.Context, string, *domain.Secret) error {
			return errors.New("сбой БД")
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodPost, "/api/v1/secrets", secretBody)
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("битый JSON", func(t *testing.T) {
		h := newSecretHandler(&stubSecrets{})
		rec := doRequest(t, h.Routes(), http.MethodPost, "/api/v1/secrets", `{not-json`)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("без авторизации", func(t *testing.T) {
		h := newSecretHandler(&stubSecrets{})
		rec := doRequestWithAuth(t, h.Routes(), http.MethodPost, "/api/v1/secrets", secretBody, false)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestHandleListSecrets(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		secrets := &stubSecrets{listFn: func(context.Context, string) ([]*domain.Secret, error) {
			return []*domain.Secret{testSecret()}, nil
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/secrets", "")

		require.Equal(t, http.StatusOK, rec.Code)
		var resp listResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp.Secrets, 1)
	})

	t.Run("внутренняя ошибка", func(t *testing.T) {
		secrets := &stubSecrets{listFn: func(context.Context, string) ([]*domain.Secret, error) {
			return nil, errors.New("сбой")
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/secrets", "")
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("без авторизации", func(t *testing.T) {
		h := newSecretHandler(&stubSecrets{})
		rec := doRequestWithAuth(t, h.Routes(), http.MethodGet, "/api/v1/secrets", "", false)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestHandleGetSecret(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		secrets := &stubSecrets{getFn: func(_ context.Context, userID, id string) (*domain.Secret, error) {
			require.Equal(t, "user-1", userID)
			require.Equal(t, "secret-1", id)
			return testSecret(), nil
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/secrets/secret-1", "")

		require.Equal(t, http.StatusOK, rec.Code)
		var resp domain.Secret
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, "secret-1", resp.ID)
	})

	t.Run("не найдено", func(t *testing.T) {
		secrets := &stubSecrets{getFn: func(context.Context, string, string) (*domain.Secret, error) {
			return nil, domain.ErrSecretNotFound
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/secrets/absent", "")
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("чужой секрет отдаётся как не найденный", func(t *testing.T) {
		// Сервис не отличает чужой секрет от несуществующего и возвращает
		// domain.ErrSecretNotFound — наружу уходит 404, а не 403.
		secrets := &stubSecrets{getFn: func(context.Context, string, string) (*domain.Secret, error) {
			return nil, domain.ErrSecretNotFound
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/secrets/secret-1", "")
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("внутренняя ошибка", func(t *testing.T) {
		secrets := &stubSecrets{getFn: func(context.Context, string, string) (*domain.Secret, error) {
			return nil, errors.New("сбой")
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/secrets/secret-1", "")
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandleUpdateSecret(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		var gotID string
		secrets := &stubSecrets{updateFn: func(_ context.Context, userID string, s *domain.Secret) error {
			gotID = s.ID
			require.Equal(t, "user-1", userID)
			return nil
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodPut, "/api/v1/secrets/secret-1", secretBody)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "secret-1", gotID)
	})

	t.Run("конфликт версий", func(t *testing.T) {
		secrets := &stubSecrets{updateFn: func(context.Context, string, *domain.Secret) error {
			return domain.ErrSecretVersionMismatch
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodPut, "/api/v1/secrets/secret-1", secretBody)
		require.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("чужой секрет отдаётся как не найденный", func(t *testing.T) {
		// Сервис не отличает чужой секрет от несуществующего и возвращает
		// domain.ErrSecretNotFound — наружу уходит 404, а не 403.
		secrets := &stubSecrets{updateFn: func(context.Context, string, *domain.Secret) error {
			return domain.ErrSecretNotFound
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodPut, "/api/v1/secrets/secret-1", secretBody)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("валидация", func(t *testing.T) {
		secrets := &stubSecrets{updateFn: func(context.Context, string, *domain.Secret) error {
			return domain.ErrInvalidSecretData
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodPut, "/api/v1/secrets/secret-1", secretBody)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("не найдено", func(t *testing.T) {
		secrets := &stubSecrets{updateFn: func(context.Context, string, *domain.Secret) error {
			return domain.ErrSecretNotFound
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodPut, "/api/v1/secrets/absent", secretBody)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("битый JSON", func(t *testing.T) {
		h := newSecretHandler(&stubSecrets{})
		rec := doRequest(t, h.Routes(), http.MethodPut, "/api/v1/secrets/secret-1", `nope`)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestHandleDeleteSecret(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		var gotID, gotUserID string
		secrets := &stubSecrets{deleteFn: func(_ context.Context, userID, id string) error {
			gotUserID, gotID = userID, id
			return nil
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodDelete, "/api/v1/secrets/secret-1", "")

		require.Equal(t, http.StatusNoContent, rec.Code)
		require.Equal(t, "secret-1", gotID)
		require.Equal(t, "user-1", gotUserID)
	})

	t.Run("не найдено", func(t *testing.T) {
		secrets := &stubSecrets{deleteFn: func(context.Context, string, string) error {
			return domain.ErrSecretNotFound
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodDelete, "/api/v1/secrets/absent", "")
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("чужой секрет отдаётся как не найденный", func(t *testing.T) {
		// Сервис не отличает чужой секрет от несуществующего и возвращает
		// domain.ErrSecretNotFound — наружу уходит 404, а не 403.
		secrets := &stubSecrets{deleteFn: func(context.Context, string, string) error {
			return domain.ErrSecretNotFound
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodDelete, "/api/v1/secrets/secret-1", "")
		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestHandleSync(t *testing.T) {
	t.Run("без since использует список", func(t *testing.T) {
		secrets := &stubSecrets{
			listFn: func(context.Context, string) ([]*domain.Secret, error) {
				return []*domain.Secret{testSecret()}, nil
			},
			syncFn: func(context.Context, string, time.Time) ([]*domain.Secret, error) {
				t.Fatal("Sync не должен вызываться без параметра since")
				return nil, nil
			},
		}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/sync", "")
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("с since", func(t *testing.T) {
		var gotSince time.Time
		secrets := &stubSecrets{syncFn: func(_ context.Context, userID string, since time.Time) ([]*domain.Secret, error) {
			require.Equal(t, "user-1", userID)
			gotSince = since
			return []*domain.Secret{testSecret()}, nil
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/sync?since=2026-01-15T10:30:00Z", "")

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC), gotSince)
	})

	t.Run("некорректный since", func(t *testing.T) {
		h := newSecretHandler(&stubSecrets{})
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/sync?since=15-01-2026", "")
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ошибка синхронизации", func(t *testing.T) {
		secrets := &stubSecrets{syncFn: func(context.Context, string, time.Time) ([]*domain.Secret, error) {
			return nil, errors.New("сбой")
		}}
		h := newSecretHandler(secrets)
		rec := doRequest(t, h.Routes(), http.MethodGet, "/api/v1/sync?since=2026-01-15T10:30:00Z", "")
		require.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestParseSinceParam(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    time.Time
		wantErr bool
	}{
		{
			name: "с Z",
			raw:  "2026-01-15T10:30:00Z",
			want: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name: "с положительным смещением",
			raw:  "2026-01-15T13:30:00+03:00",
			want: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name: "с отрицательным смещением",
			raw:  "2026-01-15T05:30:00-05:00",
			want: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:    "пустая строка",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "неверный формат",
			raw:     "15.01.2026 10:30",
			wantErr: true,
		},
		{
			name:    "только дата",
			raw:     "2026-01-15",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSinceParam(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.True(t, tt.want.Equal(got), "ожидалось %v, получено %v", tt.want, got)
		})
	}
}

func TestStatusForSecretErrors(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{domain.ErrForbidden, http.StatusForbidden},
		{domain.ErrSecretVersionMismatch, http.StatusConflict},
		{domain.ErrSecretAlreadyExists, http.StatusConflict},
		{domain.ErrInvalidSecretData, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			require.Equal(t, tt.want, statusForError(tt.err))
		})
	}
}
