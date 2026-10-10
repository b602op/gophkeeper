package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/b602op/gophkeeper/internal/domain"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := New(srv.URL)
	client.maxRetries = 0
	client.retryBackoff = time.Millisecond
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		t.Errorf("Encode вернул ошибку: %v", err)
	}
}

func TestRegister(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/register" {
			t.Errorf("неожиданный запрос: %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, http.StatusCreated, registerResponse{ID: "user-1", Login: "alice"})
	})

	user, err := client.Register(context.Background(), "alice", "secret")
	if err != nil {
		t.Fatalf("Register вернул ошибку: %v", err)
	}
	if user.ID != "user-1" || user.Login != "alice" {
		t.Fatalf("Register вернул %+v", user)
	}
}

func TestLogin(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/login" {
			t.Errorf("неожиданный запрос: %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, http.StatusOK, loginResponse{Token: "jwt-token"})
	})

	token, err := client.Login(context.Background(), "alice", "secret")
	if err != nil {
		t.Fatalf("Login вернул ошибку: %v", err)
	}
	if token != "jwt-token" {
		t.Fatalf("Login вернул %q", token)
	}
}

func TestLoginWithoutToken(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, loginResponse{})
	})

	if _, err := client.Login(context.Background(), "alice", "secret"); err == nil {
		t.Fatal("ожидалась ошибка при отсутствии токена")
	}
}

func TestSecretCRUD(t *testing.T) {
	secret := &domain.Secret{
		ID:      "id-1",
		UserID:  "user-1",
		Type:    domain.SecretTypeText,
		Name:    "заметка",
		Data:    []byte{0x01},
		Version: 1,
	}

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/secrets":
			writeJSON(t, w, http.StatusCreated, secret)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/secrets/id-1":
			writeJSON(t, w, http.StatusOK, secret)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/secrets":
			writeJSON(t, w, http.StatusOK, listResponse{Secrets: []*domain.Secret{secret}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/secrets/id-1":
			writeJSON(t, w, http.StatusOK, secret)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/secrets/id-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("неожиданный запрос: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	ctx := context.Background()

	created, err := client.CreateSecret(ctx, secret)
	if err != nil {
		t.Fatalf("CreateSecret вернул ошибку: %v", err)
	}
	if created.ID != "id-1" {
		t.Fatalf("CreateSecret вернул %+v", created)
	}

	got, err := client.GetSecret(ctx, "id-1")
	if err != nil {
		t.Fatalf("GetSecret вернул ошибку: %v", err)
	}
	if got.Name != "заметка" {
		t.Fatalf("GetSecret вернул %+v", got)
	}

	list, err := client.ListSecrets(ctx)
	if err != nil {
		t.Fatalf("ListSecrets вернул ошибку: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListSecrets вернул %d записей", len(list))
	}

	updated, err := client.UpdateSecret(ctx, secret)
	if err != nil {
		t.Fatalf("UpdateSecret вернул ошибку: %v", err)
	}
	if updated.ID != "id-1" {
		t.Fatalf("UpdateSecret вернул %+v", updated)
	}

	if err := client.DeleteSecret(ctx, "id-1"); err != nil {
		t.Fatalf("DeleteSecret вернул ошибку: %v", err)
	}
}

func TestSecretNilArguments(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("запрос не должен выполняться")
	})
	ctx := context.Background()

	if _, err := client.CreateSecret(ctx, nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("ошибка %v не оборачивает ErrValidation", err)
	}
	if _, err := client.UpdateSecret(ctx, nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("ошибка %v не оборачивает ErrValidation", err)
	}
}

func TestSync(t *testing.T) {
	var gotQuery url.Values
	watermark := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sync" {
			t.Errorf("неожиданный путь: %s", r.URL.Path)
		}
		gotQuery = r.URL.Query()
		writeJSON(t, w, http.StatusOK, syncResponse{
			Secrets:       []*domain.Secret{},
			SyncWatermark: watermark,
		})
	})
	ctx := context.Background()

	result, err := client.Sync(ctx, time.Time{})
	if err != nil {
		t.Fatalf("Sync без since вернул ошибку: %v", err)
	}
	if !result.Watermark.Equal(watermark) {
		t.Fatalf("watermark = %v, ожидался %v", result.Watermark, watermark)
	}
	if gotQuery.Has("since") {
		t.Fatalf("при нулевом времени параметр since не должен передаваться: %v", gotQuery)
	}

	since := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	if _, err := client.Sync(ctx, since); err != nil {
		t.Fatalf("Sync с since вернул ошибку: %v", err)
	}
	if gotQuery.Get("since") != "2026-01-15T10:30:00Z" {
		t.Fatalf("since = %q", gotQuery.Get("since"))
	}
}

func TestAuthorizationHeader(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-token" {
			t.Errorf("Authorization = %q", got)
		}
		writeJSON(t, w, http.StatusOK, listResponse{})
	})

	client.SetToken("jwt-token")
	if client.Token() != "jwt-token" {
		t.Fatalf("Token = %q", client.Token())
	}

	if _, err := client.ListSecrets(context.Background()); err != nil {
		t.Fatalf("ListSecrets вернул ошибку: %v", err)
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"400", http.StatusBadRequest, ErrValidation},
		{"401", http.StatusUnauthorized, ErrUnauthorized},
		{"403", http.StatusForbidden, ErrForbidden},
		{"404", http.StatusNotFound, ErrNotFound},
		{"409", http.StatusConflict, ErrConflict},
		{"418", http.StatusTeapot, ErrUnexpectedStatus},
		{"500", http.StatusInternalServerError, ErrServer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, tt.status, errorResponse{Error: "текст ошибки"})
			})

			_, err := client.GetSecret(context.Background(), "id-1")
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("ошибка %v не оборачивает %v", err, tt.want)
			}
		})
	}
}

func TestErrorMessageFallback(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := io.WriteString(w, "не JSON"); err != nil {
			t.Errorf("WriteString вернул ошибку: %v", err)
		}
	})

	_, err := client.GetSecret(context.Background(), "id-1")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("ошибка %v не оборачивает ErrValidation", err)
	}
}

func TestRetryOnServerError(t *testing.T) {
	var calls atomic.Int32

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			writeJSON(t, w, http.StatusInternalServerError, errorResponse{Error: "сбой"})
			return
		}
		writeJSON(t, w, http.StatusOK, listResponse{Secrets: []*domain.Secret{}})
	})
	client.maxRetries = 2

	if _, err := client.ListSecrets(context.Background()); err != nil {
		t.Fatalf("ListSecrets вернул ошибку: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("запросов = %d, ожидалось 3", got)
	}
}

func TestRetryExhausted(t *testing.T) {
	var calls atomic.Int32

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(t, w, http.StatusInternalServerError, errorResponse{Error: "сбой"})
	})
	client.maxRetries = 1

	_, err := client.ListSecrets(context.Background())
	if !errors.Is(err, ErrServer) {
		t.Fatalf("ошибка %v не оборачивает ErrServer", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("запросов = %d, ожидалось 2", got)
	}
}

func TestInvalidResponseJSON(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "{"); err != nil {
			t.Errorf("WriteString вернул ошибку: %v", err)
		}
	})

	if _, err := client.ListSecrets(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка разбора ответа")
	}
}

func TestNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client := New(srv.URL)
	client.maxRetries = 0

	if _, err := client.ListSecrets(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка сети")
	}
}

func TestContextCancelled(t *testing.T) {
	release := make(chan struct{})
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		<-release
	})
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.ListSecrets(ctx); err == nil {
		t.Fatal("ожидалась ошибка отменённого контекста")
	}
}

func TestRequestCreationError(t *testing.T) {
	client := New("://bad")
	client.maxRetries = 0

	if _, err := client.ListSecrets(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка создания запроса")
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	client := New("http://localhost:8080/")
	if client.baseURL != "http://localhost:8080" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
}
