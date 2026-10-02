package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/b602op/gophkeeper/internal/domain"
)

const testSecret = "test-secret-key-1234567890"

// stubUserRepo — управляемый мок репозитория пользователей.
type stubUserRepo struct {
	createFn     func(ctx context.Context, user *domain.User) error
	getByLoginFn func(ctx context.Context, login string) (*domain.User, error)
	getByIDFn    func(ctx context.Context, id string) (*domain.User, error)
}

func (s *stubUserRepo) Create(ctx context.Context, user *domain.User) error {
	if s.createFn == nil {
		return nil
	}
	return s.createFn(ctx, user)
}

func (s *stubUserRepo) GetByLogin(ctx context.Context, login string) (*domain.User, error) {
	if s.getByLoginFn == nil {
		return nil, domain.ErrUserNotFound
	}
	return s.getByLoginFn(ctx, login)
}

func (s *stubUserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	if s.getByIDFn == nil {
		return nil, domain.ErrUserNotFound
	}
	return s.getByIDFn(ctx, id)
}

// newTestAuth создаёт сервис с фиксированными временем и идентификатором.
func newTestAuth(repo UserRepository) *AuthService {
	svc := NewAuthService(repo, testSecret, time.Hour, bcrypt.MinCost)
	fixed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }
	svc.newID = func() string { return "fixed-id" }
	return svc
}

// hashPassword возвращает bcrypt-хеш пароля с минимальной стоимостью.
func hashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	return string(hash)
}

func TestAuthServiceRegister(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		var created *domain.User
		repo := &stubUserRepo{createFn: func(_ context.Context, u *domain.User) error {
			created = u
			return nil
		}}
		svc := newTestAuth(repo)

		user, err := svc.Register(context.Background(), " alice ", "password123")
		require.NoError(t, err)
		require.Equal(t, "alice", user.Login)
		require.Equal(t, "fixed-id", user.ID)
		require.Same(t, user, created)
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("password123")))
	})

	t.Run("дубликат логина", func(t *testing.T) {
		repo := &stubUserRepo{createFn: func(context.Context, *domain.User) error {
			return domain.ErrUserAlreadyExists
		}}
		svc := newTestAuth(repo)

		_, err := svc.Register(context.Background(), "alice", "password123")
		require.ErrorIs(t, err, domain.ErrUserAlreadyExists)
	})

	t.Run("ошибка репозитория", func(t *testing.T) {
		dbErr := errors.New("нет связи")
		repo := &stubUserRepo{createFn: func(context.Context, *domain.User) error { return dbErr }}
		svc := newTestAuth(repo)

		_, err := svc.Register(context.Background(), "alice", "password123")
		require.ErrorIs(t, err, dbErr)
	})

	t.Run("ошибка хеширования", func(t *testing.T) {
		repo := &stubUserRepo{}
		svc := NewAuthService(repo, testSecret, time.Hour, 99)

		_, err := svc.Register(context.Background(), "alice", "password123")
		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrValidation)
	})
}

func TestAuthServiceRegisterValidation(t *testing.T) {
	tests := []struct {
		name     string
		login    string
		password string
	}{
		{"короткий логин", "ab", "password123"},
		{"длинный логин", strings.Repeat("a", maxLoginLen+1), "password123"},
		{"недопустимые символы", "alice!", "password123"},
		{"логин с пробелом внутри", "ali ce", "password123"},
		{"короткий пароль", "alice", "short"},
		{"длинный пароль", "alice", strings.Repeat("a", maxPasswordLen+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubUserRepo{createFn: func(context.Context, *domain.User) error {
				t.Fatal("репозиторий не должен вызываться при невалидных данных")
				return nil
			}}
			svc := newTestAuth(repo)

			_, err := svc.Register(context.Background(), tt.login, tt.password)
			require.ErrorIs(t, err, domain.ErrValidation)
		})
	}
}

func TestAuthServiceLogin(t *testing.T) {
	t.Run("успех", func(t *testing.T) {
		repo := &stubUserRepo{getByLoginFn: func(_ context.Context, login string) (*domain.User, error) {
			require.Equal(t, "alice", login)
			return &domain.User{ID: "user-1", Login: "alice", PasswordHash: hashPassword(t, "password123")}, nil
		}}
		svc := newTestAuth(repo)

		token, err := svc.Login(context.Background(), "alice", "password123")
		require.NoError(t, err)
		require.NotEmpty(t, token)

		subject, err := svc.ParseToken(token)
		require.NoError(t, err)
		require.Equal(t, "user-1", subject)
	})

	t.Run("неизвестный пользователь", func(t *testing.T) {
		repo := &stubUserRepo{getByLoginFn: func(context.Context, string) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		}}
		svc := newTestAuth(repo)

		_, err := svc.Login(context.Background(), "bob", "password123")
		require.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("неверный пароль", func(t *testing.T) {
		repo := &stubUserRepo{getByLoginFn: func(context.Context, string) (*domain.User, error) {
			return &domain.User{ID: "user-1", Login: "alice", PasswordHash: hashPassword(t, "password123")}, nil
		}}
		svc := newTestAuth(repo)

		_, err := svc.Login(context.Background(), "alice", "wrong-password")
		require.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("ошибка репозитория", func(t *testing.T) {
		dbErr := errors.New("таймаут")
		repo := &stubUserRepo{getByLoginFn: func(context.Context, string) (*domain.User, error) {
			return nil, dbErr
		}}
		svc := newTestAuth(repo)

		_, err := svc.Login(context.Background(), "alice", "password123")
		require.ErrorIs(t, err, dbErr)
		require.NotErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("пустые учётные данные", func(t *testing.T) {
		svc := newTestAuth(&stubUserRepo{})
		_, err := svc.Login(context.Background(), "  ", "password123")
		require.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})
}

func TestAuthServiceParseToken(t *testing.T) {
	repo := &stubUserRepo{}
	svc := newTestAuth(repo)

	t.Run("невалидная строка", func(t *testing.T) {
		_, err := svc.ParseToken("not-a-token")
		require.ErrorIs(t, err, domain.ErrInvalidToken)
	})

	t.Run("чужая подпись", func(t *testing.T) {
		other := NewAuthService(repo, "another-secret-key-1234567", time.Hour, bcrypt.MinCost)
		other.now = svc.now
		token, err := other.generateToken(&domain.User{ID: "user-1"})
		require.NoError(t, err)

		_, err = svc.ParseToken(token)
		require.ErrorIs(t, err, domain.ErrInvalidToken)
	})

	t.Run("истёкший токен", func(t *testing.T) {
		expired := newTestAuth(repo)
		expired.tokenTTL = -time.Hour
		token, err := expired.generateToken(&domain.User{ID: "user-1"})
		require.NoError(t, err)

		_, err = expired.ParseToken(token)
		require.ErrorIs(t, err, domain.ErrInvalidToken)
	})

	t.Run("неподдерживаемый алгоритм", func(t *testing.T) {
		claims := jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(svc.now().Add(time.Hour)),
		}
		raw, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		_, err = svc.ParseToken(raw)
		require.ErrorIs(t, err, domain.ErrInvalidToken)
	})

	t.Run("пустой субъект", func(t *testing.T) {
		claims := jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(svc.now().Add(time.Hour))}
		raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
		require.NoError(t, err)

		_, err = svc.ParseToken(raw)
		require.ErrorIs(t, err, domain.ErrInvalidToken)
	})
}
