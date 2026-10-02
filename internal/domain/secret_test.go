package domain

import "testing"

func TestSecretTypeValid(t *testing.T) {
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
			if got := tt.typ.Valid(); got != tt.want {
				t.Fatalf("Valid() = %v, ожидалось %v", got, tt.want)
			}
		})
	}
}

func TestDomainErrorsAreDistinct(t *testing.T) {
	all := []error{
		ErrUserNotFound,
		ErrUserAlreadyExists,
		ErrInvalidCredentials,
		ErrInvalidToken,
		ErrSecretNotFound,
		ErrInvalidSecretType,
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
