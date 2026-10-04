package session

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func newTestSession(t *testing.T) *Session {
	t.Helper()
	return NewWithTokenPath(filepath.Join(t.TempDir(), "token"))
}

func TestSetAndGetToken(t *testing.T) {
	s := newTestSession(t)

	if err := s.SetToken("jwt-token"); err != nil {
		t.Fatalf("SetToken вернул ошибку: %v", err)
	}

	raw, err := os.ReadFile(s.tokenPath)
	if err != nil {
		t.Fatalf("ReadFile вернул ошибку: %v", err)
	}
	if string(raw) != "jwt-token" {
		t.Fatalf("содержимое токена = %q, ожидалось %q", raw, "jwt-token")
	}

	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(s.tokenPath)
		if statErr != nil {
			t.Fatalf("Stat вернул ошибку: %v", statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("права токена = %v, ожидалось 0600", info.Mode().Perm())
		}
	}

	got, err := s.GetToken()
	if err != nil {
		t.Fatalf("GetToken вернул ошибку: %v", err)
	}
	if got != "jwt-token" {
		t.Fatalf("GetToken = %q, ожидалось %q", got, "jwt-token")
	}
}

func TestGetTokenNoToken(t *testing.T) {
	s := newTestSession(t)

	if _, err := s.GetToken(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("ошибка %v не оборачивает ErrNoToken", err)
	}
}

func TestGetTokenFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("  saved-token  \n"), 0o600); err != nil {
		t.Fatalf("WriteFile вернул ошибку: %v", err)
	}

	s := NewWithTokenPath(path)
	got, err := s.GetToken()
	if err != nil {
		t.Fatalf("GetToken вернул ошибку: %v", err)
	}
	if got != "saved-token" {
		t.Fatalf("GetToken = %q, ожидалось %q", got, "saved-token")
	}
}

func TestGetTokenEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("WriteFile вернул ошибку: %v", err)
	}

	s := NewWithTokenPath(path)
	if _, err := s.GetToken(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("ошибка %v не оборачивает ErrNoToken", err)
	}
}

func TestGetTokenReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir вернул ошибку: %v", err)
	}

	s := NewWithTokenPath(path)
	_, err := s.GetToken()
	if err == nil {
		t.Fatal("ожидалась ошибка чтения")
	}
	if errors.Is(err, ErrNoToken) {
		t.Fatal("ошибка чтения не должна трактоваться как отсутствие токена")
	}
}

func TestSetTokenEmpty(t *testing.T) {
	s := newTestSession(t)

	if err := s.SetToken("   "); err == nil {
		t.Fatal("ожидалась ошибка при пустом токене")
	}
}

func TestSetTokenCreatesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "token")
	s := NewWithTokenPath(path)

	if err := s.SetToken("token"); err != nil {
		t.Fatalf("SetToken вернул ошибку: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("токен не создан: %v", err)
	}
}

func TestMasterKeyLifecycle(t *testing.T) {
	s := newTestSession(t)

	if _, err := s.GetMasterKey(); !errors.Is(err, ErrNoMasterKey) {
		t.Fatalf("ошибка %v не оборачивает ErrNoMasterKey", err)
	}

	now := time.Now()
	s.now = func() time.Time { return now }
	s.SetMasterKey([]byte("master-key"))

	got, err := s.GetMasterKey()
	if err != nil {
		t.Fatalf("GetMasterKey вернул ошибку: %v", err)
	}
	if string(got) != "master-key" {
		t.Fatalf("GetMasterKey = %q, ожидалось %q", got, "master-key")
	}

	// В пределах TTL ключ ещё доступен.
	now = now.Add(masterKeyTTL - time.Second)
	if _, err := s.GetMasterKey(); err != nil {
		t.Fatalf("ключ должен быть доступен до истечения TTL: %v", err)
	}

	// После истечения TTL ключ затирается.
	now = now.Add(2 * time.Second)
	if _, err := s.GetMasterKey(); !errors.Is(err, ErrNoMasterKey) {
		t.Fatalf("ошибка %v не оборачивает ErrNoMasterKey", err)
	}
}

func TestMasterKeyIsCopied(t *testing.T) {
	s := newTestSession(t)
	s.now = time.Now

	original := []byte("master-key")
	s.SetMasterKey(original)
	original[0] = 'X'

	got, err := s.GetMasterKey()
	if err != nil {
		t.Fatalf("GetMasterKey вернул ошибку: %v", err)
	}
	if string(got) != "master-key" {
		t.Fatalf("изменение исходного среза повлияло на ключ: %q", got)
	}

	got[0] = 'Y'
	again, err := s.GetMasterKey()
	if err != nil {
		t.Fatalf("GetMasterKey вернул ошибку: %v", err)
	}
	if string(again) != "master-key" {
		t.Fatalf("изменение возвращённого среза повлияло на ключ: %q", again)
	}
}

func TestClear(t *testing.T) {
	s := newTestSession(t)
	s.now = time.Now

	if err := s.SetToken("token"); err != nil {
		t.Fatalf("SetToken вернул ошибку: %v", err)
	}
	s.SetMasterKey([]byte("master-key"))

	if err := s.Clear(); err != nil {
		t.Fatalf("Clear вернул ошибку: %v", err)
	}
	if _, err := os.Stat(s.tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("файл токена не удалён: %v", err)
	}
	if _, err := s.GetToken(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("ошибка %v не оборачивает ErrNoToken", err)
	}
	if _, err := s.GetMasterKey(); !errors.Is(err, ErrNoMasterKey) {
		t.Fatalf("ошибка %v не оборачивает ErrNoMasterKey", err)
	}
}

func TestClearIdempotent(t *testing.T) {
	s := newTestSession(t)

	if err := s.Clear(); err != nil {
		t.Fatalf("первый Clear вернул ошибку: %v", err)
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("повторный Clear вернул ошибку: %v", err)
	}
}

func TestSaltLifecycle(t *testing.T) {
	s := newTestSession(t)

	if _, err := s.GetSalt(); !errors.Is(err, ErrNoMasterKey) {
		t.Fatalf("ошибка %v не оборачивает ErrNoMasterKey", err)
	}

	now := time.Now()
	s.now = func() time.Time { return now }
	s.SetSalt([]byte("0123456789abcdef"))

	got, err := s.GetSalt()
	if err != nil {
		t.Fatalf("GetSalt вернул ошибку: %v", err)
	}
	if string(got) != "0123456789abcdef" {
		t.Fatalf("GetSalt = %q", got)
	}

	now = now.Add(masterKeyTTL + time.Second)
	if _, err := s.GetSalt(); !errors.Is(err, ErrNoMasterKey) {
		t.Fatalf("ошибка %v не оборачивает ErrNoMasterKey", err)
	}
}

func TestUserID(t *testing.T) {
	s := newTestSession(t)

	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user-42"}`))
	if err := s.SetToken("header." + payload + ".signature"); err != nil {
		t.Fatalf("SetToken вернул ошибку: %v", err)
	}

	userID, err := s.UserID()
	if err != nil {
		t.Fatalf("UserID вернул ошибку: %v", err)
	}
	if userID != "user-42" {
		t.Fatalf("UserID = %q, ожидалось %q", userID, "user-42")
	}
}

func TestUserIDInvalid(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"без частей", "token"},
		{"битый base64", "header.!!!.sig"},
		{"не JSON", "header." + base64.RawURLEncoding.EncodeToString([]byte("nope")) + ".sig"},
		{"без субъекта", "header." + base64.RawURLEncoding.EncodeToString([]byte(`{}`)) + ".sig"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSession(t)
			if err := s.SetToken(tt.token); err != nil {
				t.Fatalf("SetToken вернул ошибку: %v", err)
			}
			if _, err := s.UserID(); err == nil {
				t.Fatal("ожидалась ошибка")
			}
		})
	}
}

func TestUserIDNoToken(t *testing.T) {
	s := newTestSession(t)

	if _, err := s.UserID(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("ошибка %v не оборачивает ErrNoToken", err)
	}
}
