// Package api реализует HTTP-клиент к серверу GophKeeper.
//
// Клиент не содержит бизнес-логики: он отвечает за сериализацию, заголовок
// авторизации, повторные попытки при временных сбоях и перевод HTTP-статусов в
// ошибки, понятные остальному коду. Тело запроса и ответа — те же доменные
// модели, что использует сервер.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/b602op/gophkeeper/internal/domain"
)

// Параметры HTTP-клиента.
const (
	// defaultTimeout — общий таймаут одного запроса.
	defaultTimeout = 30 * time.Second
	// defaultMaxRetries — число повторных попыток при 5xx.
	defaultMaxRetries = 3
	// defaultRetryBackoff — базовый интервал между попытками.
	defaultRetryBackoff = 200 * time.Millisecond
	// maxResponseSize — ограничение размера ответа сервера.
	maxResponseSize = 32 << 20
)

// Ошибки пакета.
//
// Ошибки HTTP-статусов совпадают с доменными, чтобы вызывающий код мог
// проверять их единообразно через errors.Is. ErrServer и ErrUnexpectedStatus
// описывают сбои транспорта и неожиданные ответы.
var (
	// ErrUnauthorized соответствует HTTP 401.
	ErrUnauthorized = domain.ErrInvalidToken
	// ErrForbidden соответствует HTTP 403.
	ErrForbidden = domain.ErrForbidden
	// ErrNotFound соответствует HTTP 404.
	ErrNotFound = domain.ErrSecretNotFound
	// ErrConflict соответствует HTTP 409.
	ErrConflict = domain.ErrSecretVersionMismatch
	// ErrValidation соответствует HTTP 400.
	ErrValidation = domain.ErrValidation
	// ErrServer соответствует ответу 5xx, после которого попытки исчерпаны.
	ErrServer = errors.New("сервер вернул ошибку")
	// ErrUnexpectedStatus описывает прочие неожиданные статусы.
	ErrUnexpectedStatus = errors.New("неожиданный статус ответа сервера")
)

// Client — HTTP-клиент к серверу GophKeeper.
type Client struct {
	baseURL string
	http    *http.Client

	maxRetries   int
	retryBackoff time.Duration

	mu    sync.RWMutex
	token string
}

// New создаёт клиент для указанного базового адреса сервера.
func New(baseURL string) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		http:         &http.Client{Timeout: defaultTimeout},
		maxRetries:   defaultMaxRetries,
		retryBackoff: defaultRetryBackoff,
	}
}

// SetToken устанавливает JWT, добавляемый в заголовок Authorization.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

// Token возвращает текущий JWT.
func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// do выполняет HTTP-запрос с повторными попытками при 5xx.
//
// Тело сериализуется один раз до цикла; при повторе оно передаётся заново.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("сериализация запроса: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.retryBackoff * time.Duration(1<<(attempt-1))
			if err := sleep(ctx, backoff); err != nil {
				return err
			}
		}

		err := c.attempt(ctx, method, path, payload, body != nil, out)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrServer) {
			lastErr = err
			continue
		}
		return err
	}
	return lastErr
}

// attempt выполняет одну попытку запроса.
func (c *Client) attempt(ctx context.Context, method, path string, payload []byte, hasBody bool, out any) error {
	var reader io.Reader
	if hasBody {
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("создание запроса: %w", err)
	}
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := c.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("выполнение запроса: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return handleResponse(resp, out)
}

// handleResponse проверяет статус и разбирает тело ответа.
func handleResponse(resp *http.Response, out any) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		if out == nil || resp.StatusCode == http.StatusNoContent {
			return nil
		}
		decoder := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize))
		if err := decoder.Decode(out); err != nil {
			return fmt.Errorf("разбор ответа: %w", err)
		}
		return nil
	}

	return statusError(resp.StatusCode, readErrorMessage(resp.Body))
}

// statusError переводит HTTP-статус в ошибку пакета.
func statusError(status int, message string) error {
	base := baseError(status)
	if message == "" {
		return base
	}
	return fmt.Errorf("%w: %s", base, message)
}

// baseError сопоставляет статус с базовой ошибкой.
func baseError(status int) error {
	switch status {
	case http.StatusBadRequest:
		return ErrValidation
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	default:
		if status >= http.StatusInternalServerError {
			return ErrServer
		}
		return fmt.Errorf("%w: %d", ErrUnexpectedStatus, status)
	}
}

// errorResponse — единый формат ошибки сервера.
type errorResponse struct {
	Error string `json:"error"`
}

// readErrorMessage извлекает текст ошибки из тела ответа.
func readErrorMessage(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, 64<<10))
	if err != nil {
		return ""
	}
	var parsed errorResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Error)
}

// sleep ждёт указанное время или завершается вместе с контекстом.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
