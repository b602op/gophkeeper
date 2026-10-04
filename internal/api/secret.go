package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/b602op/gophkeeper/internal/domain"
)

// credentialsRequest — тело запросов регистрации и входа.
type credentialsRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// registerResponse — ответ на успешную регистрацию.
type registerResponse struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

// loginResponse — ответ на успешный вход.
type loginResponse struct {
	Token string `json:"token"`
}

// listResponse — ответ со списком секретов.
type listResponse struct {
	Secrets []*domain.Secret `json:"secrets"`
}

// Register создаёт пользователя на сервере.
//
// Сервер не выдаёт токен при регистрации, поэтому после успешного вызова
// клиент выполняет вход.
func (c *Client) Register(ctx context.Context, login, password string) (*domain.User, error) {
	req := credentialsRequest{Login: login, Password: password}

	var resp registerResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/register", req, &resp); err != nil {
		return nil, fmt.Errorf("регистрация: %w", err)
	}
	return &domain.User{ID: resp.ID, Login: resp.Login}, nil
}

// Login выполняет вход и возвращает JWT.
func (c *Client) Login(ctx context.Context, login, password string) (string, error) {
	req := credentialsRequest{Login: login, Password: password}

	var resp loginResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/login", req, &resp); err != nil {
		return "", fmt.Errorf("вход: %w", err)
	}
	if resp.Token == "" {
		return "", fmt.Errorf("вход: сервер не вернул токен")
	}
	return resp.Token, nil
}

// CreateSecret создаёт секрет и возвращает его серверную версию.
func (c *Client) CreateSecret(ctx context.Context, secret *domain.Secret) (*domain.Secret, error) {
	if secret == nil {
		return nil, fmt.Errorf("%w: секрет не задан", ErrValidation)
	}

	var resp domain.Secret
	if err := c.do(ctx, http.MethodPost, "/api/v1/secrets", secret, &resp); err != nil {
		return nil, fmt.Errorf("создание секрета: %w", err)
	}
	return &resp, nil
}

// GetSecret возвращает секрет по идентификатору.
func (c *Client) GetSecret(ctx context.Context, id string) (*domain.Secret, error) {
	var resp domain.Secret
	if err := c.do(ctx, http.MethodGet, "/api/v1/secrets/"+url.PathEscape(id), nil, &resp); err != nil {
		return nil, fmt.Errorf("получение секрета: %w", err)
	}
	return &resp, nil
}

// ListSecrets возвращает неудалённые секреты пользователя.
func (c *Client) ListSecrets(ctx context.Context) ([]*domain.Secret, error) {
	var resp listResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/secrets", nil, &resp); err != nil {
		return nil, fmt.Errorf("список секретов: %w", err)
	}
	return resp.Secrets, nil
}

// UpdateSecret обновляет секрет с проверкой версии на сервере.
func (c *Client) UpdateSecret(ctx context.Context, secret *domain.Secret) (*domain.Secret, error) {
	if secret == nil {
		return nil, fmt.Errorf("%w: секрет не задан", ErrValidation)
	}

	var resp domain.Secret
	path := "/api/v1/secrets/" + url.PathEscape(secret.ID)
	if err := c.do(ctx, http.MethodPut, path, secret, &resp); err != nil {
		return nil, fmt.Errorf("обновление секрета: %w", err)
	}
	return &resp, nil
}

// DeleteSecret помечает секрет удалённым на сервере.
func (c *Client) DeleteSecret(ctx context.Context, id string) error {
	path := "/api/v1/secrets/" + url.PathEscape(id)
	if err := c.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("удаление секрета: %w", err)
	}
	return nil
}

// Sync возвращает секреты, изменённые после указанного момента.
//
// Нулевое время означает запрос всех неудалённых секретов.
func (c *Client) Sync(ctx context.Context, since time.Time) ([]*domain.Secret, error) {
	path := "/api/v1/sync"
	if !since.IsZero() {
		query := url.Values{"since": []string{since.UTC().Format(time.RFC3339)}}
		path += "?" + query.Encode()
	}

	var resp listResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, fmt.Errorf("синхронизация: %w", err)
	}
	return resp.Secrets, nil
}
