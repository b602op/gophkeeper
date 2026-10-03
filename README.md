# GophKeeper

Клиент-серверная система для надёжного и безопасного хранения приватной
информации: пар логин/пароль, произвольного текста, бинарных данных и данных
банковских карт. Поддерживает произвольную текстовую метаинформацию для любой
записи (сайт, личность, банк, OTP и т.д.).

## Возможности

- Регистрация, аутентификация и авторизация пользователей по JWT.
- Хранение приватных данных пользователя на сервере (PostgreSQL).
- Синхронизация данных между несколькими клиентами одного владельца.
- Передача приватных данных владельцу по запросу.
- CLI-клиент для Windows, Linux и macOS с отображением версии и даты сборки.
- Клиентское шифрование секретов (AES-GCM, ключ из мастер-пароля) — сервер
  хранит только шифротекст (zero-knowledge).

## Архитектура

Слоистая архитектура, зависимости направлены строго в одну сторону:

```
handlers (HTTP/gRPC/CLI) -> service -> repository -> domain
```

- `internal/domain` — модели и доменные ошибки.
- `internal/config` — конфигурация: флаги > env > файл > дефолты.
- `internal/repository` — доступ к PostgreSQL через pgx (stdlib) и миграции goose.
- `internal/service` — бизнес-логика (фасад для всех транспортов).
- `internal/handler` — HTTP-хендлеры.
- `internal/middleware` — JWT-авторизация, логирование, recovery.
- `internal/logger` — структурированное логирование на `log/slog`.

## Быстрый старт

1. Поднять PostgreSQL для разработки:

   ```bash
   docker compose up -d
   ```

2. Создать конфигурацию и задать секрет JWT:

   ```bash
   cp .env.example .env
   cp config.example.json config.json
   # заполните jwt_secret (обязательное поле)
   ```

3. Применить миграции и запустить сервер:

   ```bash
   make migrate-up
   make run-server
   ```

## Конфигурация

Приоритет источников: **флаги > переменные окружения > JSON-файл > значения по
умолчанию**. Пример всех опций — в `config.example.json`.

| Флаг | Переменная окружения | Поле JSON | Назначение |
| --- | --- | --- | --- |
| `-a` | `GOPHKEEPER_RUN_ADDRESS` | `run_address` | адрес прослушивания |
| `-d` | `GOPHKEEPER_DATABASE_DSN` | `database_dsn` | DSN PostgreSQL |
| `-jwt-secret` | `GOPHKEEPER_JWT_SECRET` | `jwt_secret` | секрет подписи JWT |
| `-token-ttl` | `GOPHKEEPER_TOKEN_TTL` | `token_ttl` | время жизни токена |
| `-log-level` | `GOPHKEEPER_LOG_LEVEL` | `log_level` | уровень логирования |
| `-https` | `GOPHKEEPER_ENABLE_HTTPS` | `enable_https` | включить HTTPS |
| `-tls-cert` | `GOPHKEEPER_TLS_CERT_FILE` | `tls_cert_file` | путь к сертификату |
| `-tls-key` | `GOPHKEEPER_TLS_KEY_FILE` | `tls_key_file` | путь к ключу |
| `-bcrypt-cost` | `GOPHKEEPER_BCRYPT_COST` | `bcrypt_cost` | стоимость bcrypt |
| `-migrations` | `GOPHKEEPER_MIGRATIONS_DIR` | `migrations_dir` | каталог миграций |

Секрет JWT обязателен: без него сервер завершается с понятной ошибкой.

## API

Все защищённые маршруты требуют заголовок `Authorization: Bearer <JWT>`.
Ошибки возвращаются в едином формате `{"error": "..."}`.

| Метод | Путь | Описание |
| --- | --- | --- |
| `POST` | `/api/v1/register` | регистрация пользователя |
| `POST` | `/api/v1/login` | аутентификация, выдача JWT |
| `GET` | `/api/v1/secrets` | список неудалённых секретов |
| `GET` | `/api/v1/secrets/{id}` | получить секрет |
| `POST` | `/api/v1/secrets` | создать секрет |
| `PUT` | `/api/v1/secrets/{id}` | обновить секрет |
| `DELETE` | `/api/v1/secrets/{id}` | удалить секрет (soft delete) |
| `GET` | `/api/v1/sync?since=...` | синхронизация изменений |

### Секреты

Тело запроса создания и обновления:

```json
{
  "type": "credentials",
  "name": "Почта",
  "metadata": "личное",
  "data": "<base64 от шифротекста>",
  "version": 1
}
```

- `type` — `credentials`, `text`, `binary` или `card`.
- `data` — зашифрованные на клиенте данные (сервер видит только шифротекст).
- `metadata` — произвольная текстовая метаинформация (до 64 КиБ).
- `data` — до 1 МиБ, `name` — до 255 символов.

Обновление использует оптимистичную блокировку по `version`: если запись уже
изменена другим клиентом, сервер отвечает `409 Conflict` с ошибкой
`ErrSecretVersionMismatch`.

### Синхронизация

`GET /api/v1/sync?since=<RFC3339>` возвращает все записи, изменённые строго
после указанного момента, **включая удалённые** (`deleted_at != nil`) — так
клиенты узнают об удалениях. Без параметра `since` возвращаются все неудалённые
секреты. Пример: `?since=2026-01-15T10:30:00Z`.

### Коды ошибок

| Ошибка | HTTP |
| --- | --- |
| `ErrValidation`, `ErrInvalidSecretType`, `ErrInvalidSecretData` | 400 |
| `ErrInvalidCredentials`, `ErrInvalidToken` | 401 |
| `ErrForbidden` | 403 |
| `ErrUserAlreadyExists`, `ErrSecretAlreadyExists`, `ErrSecretVersionMismatch` | 409 |
| `ErrUserNotFound`, `ErrSecretNotFound` | 404 |

## CLI

```bash
gophkeeper register          # регистрация
gophkeeper login             # аутентификация
gophkeeper add credentials   # добавить логин/пароль
gophkeeper add card          # добавить карту
gophkeeper add text          # добавить текст
gophkeeper add binary        # добавить бинарные данные
gophkeeper get <id>          # получить секрет
gophkeeper list              # список всех записей
gophkeeper sync              # синхронизация
gophkeeper version           # версия и дата сборки
```

## Разработка

```bash
make build    # собрать сервер и клиент
make test     # тесты
make cover    # покрытие и HTML-отчёт
make lint     # go vet + staticlint
make check    # build + test + lint
make certs    # самоподписанные TLS-сертификаты
```

Покрытие юнит-тестами — не менее 70% для каждого пакета.
