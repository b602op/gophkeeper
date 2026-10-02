package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/b602op/gophkeeper/internal/domain"
)

// Ограничения на входные данные регистрации.
const (
	minLoginLen    = 3
	maxLoginLen    = 64
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt учитывает только первые 72 байта пароля.
)

// loginPattern разрешает латиницу, цифры и символы _ . -
var loginPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// AuthService реализует регистрацию, аутентификацию и работу с JWT.
type AuthService struct {
	users      UserRepository
	secret     []byte
	tokenTTL   time.Duration
	bcryptCost int

	// now и newID внедряются для детерминированных тестов.
	now   func() time.Time
	newID func() string
}

// NewAuthService создаёт сервис аутентификации.
//
// secret обязан быть непустым: это гарантирует конфигурация, которая завершает
// работу сервиса при отсутствии секрета.
func NewAuthService(users UserRepository, secret string, tokenTTL time.Duration, bcryptCost int) *AuthService {
	return &AuthService{
		users:      users,
		secret:     []byte(secret),
		tokenTTL:   tokenTTL,
		bcryptCost: bcryptCost,
		now:        time.Now,
		newID:      uuid.NewString,
	}
}

// Register создаёт нового пользователя.
//
// Логин и пароль валидируются, пароль хешируется bcrypt. При занятом логине
// возвращает domain.ErrUserAlreadyExists.
func (s *AuthService) Register(ctx context.Context, login, password string) (*domain.User, error) {
	login = strings.TrimSpace(login)
	if err := validateCredentials(login, password); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("хеширование пароля пользователя %q: %w", login, err)
	}

	now := s.now()
	user := &domain.User{
		ID:           s.newID(),
		Login:        login,
		PasswordHash: string(hash),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("регистрация пользователя %q: %w", login, err)
	}
	return user, nil
}

// Login проверяет учётные данные и возвращает подписанный JWT.
//
// При неверном логине или пароле возвращает domain.ErrInvalidCredentials без
// указания, что именно неверно, — чтобы не раскрывать существование логина.
func (s *AuthService) Login(ctx context.Context, login, password string) (string, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return "", fmt.Errorf("вход: %w", domain.ErrInvalidCredentials)
	}

	user, err := s.users.GetByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return "", fmt.Errorf("вход %q: %w", login, domain.ErrInvalidCredentials)
		}
		return "", fmt.Errorf("вход %q: %w", login, err)
	}

	if cmpErr := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); cmpErr != nil {
		return "", fmt.Errorf("вход %q: %w", login, domain.ErrInvalidCredentials)
	}

	token, err := s.generateToken(user)
	if err != nil {
		return "", fmt.Errorf("выдача токена пользователю %q: %w", login, err)
	}
	return token, nil
}

// ParseToken проверяет подпись и срок действия токена и возвращает id
// пользователя из субъекта.
//
// Любая проблема с токеном приводит к domain.ErrInvalidToken.
func (s *AuthService) ParseToken(tokenString string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("неожиданный метод подписи %v", t.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithTimeFunc(s.now))
	if err != nil {
		return "", fmt.Errorf("%w: %v", domain.ErrInvalidToken, err)
	}
	if !token.Valid {
		return "", fmt.Errorf("%w: токен не прошёл проверку", domain.ErrInvalidToken)
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("%w: в токене отсутствует субъект", domain.ErrInvalidToken)
	}
	return claims.Subject, nil
}

// generateToken выпускает подписанный HS256-токен для пользователя.
func (s *AuthService) generateToken(user *domain.User) (string, error) {
	now := s.now()
	claims := jwt.RegisteredClaims{
		Subject:   user.ID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("подпись токена: %w", err)
	}
	return signed, nil
}

// validateCredentials проверяет логин и пароль по длине и допустимым символам.
func validateCredentials(login, password string) error {
	if len(login) < minLoginLen || len(login) > maxLoginLen {
		return fmt.Errorf("%w: логин должен содержать от %d до %d символов", domain.ErrValidation, minLoginLen, maxLoginLen)
	}
	if !loginPattern.MatchString(login) {
		return fmt.Errorf("%w: логин может содержать только латинские буквы, цифры и символы _ . -", domain.ErrValidation)
	}
	if len(password) < minPasswordLen {
		return fmt.Errorf("%w: пароль должен содержать не менее %d символов", domain.ErrValidation, minPasswordLen)
	}
	if len(password) > maxPasswordLen {
		return fmt.Errorf("%w: пароль должен содержать не более %d байт", domain.ErrValidation, maxPasswordLen)
	}
	return nil
}
