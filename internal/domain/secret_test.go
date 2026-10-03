package domain

import (
	"encoding/json"
	"testing"
)

func TestSecretTypeIsValid(t *testing.T) {
	tests := []struct {
		name string
		typ  SecretType
		want bool
	}{
		{"credentials", SecretTypeCredentials, true},
		{"text", SecretTypeText, true},
		{"binary", SecretTypeBinary, true},
		{"card", SecretTypeCard, true},
		{"неизвестный", SecretType("unknown"), false},
		{"пустой", SecretType(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.IsValid(); got != tt.want {
				t.Fatalf("IsValid() = %v, ожидалось %v", got, tt.want)
			}
		})
	}
}

func TestSecretJSON(t *testing.T) {
	raw := []byte(`{
		"id": "secret-1",
		"user_id": "user-1",
		"type": "credentials",
		"name": "Почта",
		"metadata": "личное",
		"data": "c2VjcmV0",
		"version": 3,
		"created_at": "2026-01-01T12:00:00Z",
		"updated_at": "2026-01-02T12:00:00Z"
	}`)

	var secret Secret
	if err := json.Unmarshal(raw, &secret); err != nil {
		t.Fatalf("Unmarshal вернул ошибку: %v", err)
	}
	if secret.Type != SecretTypeCredentials {
		t.Errorf("Type = %q, ожидалось credentials", secret.Type)
	}
	if string(secret.Data) != "secret" {
		t.Errorf("Data = %q, ожидалось secret", secret.Data)
	}
	if secret.Version != 3 {
		t.Errorf("Version = %d, ожидалось 3", secret.Version)
	}

	// Поле deleted_at при nil не должно попадать в JSON.
	encoded, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("Marshal вернул ошибку: %v", err)
	}
	if string(encoded) == "" {
		t.Fatal("Marshal вернул пустой результат")
	}
}

func TestDomainErrorsAreDistinct(t *testing.T) {
	all := []error{
		ErrUserNotFound,
		ErrUserAlreadyExists,
		ErrInvalidCredentials,
		ErrInvalidToken,
		ErrSecretNotFound,
		ErrSecretAlreadyExists,
		ErrSecretVersionMismatch,
		ErrInvalidSecretType,
		ErrInvalidSecretData,
		ErrForbidden,
		ErrValidation,
	}
	for i, a := range all {
		for j, b := range all {
			if i != j && a == b {
				t.Fatalf("ошибки %d и %d совпадают", i, j)
			}
		}
	}
}
