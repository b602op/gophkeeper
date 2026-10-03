-- +goose Up
-- +goose StatementBegin
-- Таблица секретов. Данные хранятся только в зашифрованном на клиенте виде.
-- UUID формируется в приложении, поэтому pgcrypto не требуется.
CREATE TABLE IF NOT EXISTS secrets (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       VARCHAR(20) NOT NULL,
    name       VARCHAR(255) NOT NULL,
    metadata   TEXT NOT NULL DEFAULT '',
    data       BYTEA NOT NULL,
    version    BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_secrets_user_id ON secrets(user_id);
CREATE INDEX IF NOT EXISTS idx_secrets_user_updated ON secrets(user_id, updated_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS secrets;
-- +goose StatementEnd
