package domain

import "time"

// SecretType описывает тип хранимого секрета.
type SecretType string

// Поддерживаемые типы секретов.
const (
	// SecretTypeCredentials — пара логин/пароль.
	SecretTypeCredentials SecretType = "credentials"
	// SecretTypeText — произвольные текстовые данные.
	SecretTypeText SecretType = "text"
	// SecretTypeBinary — произвольные бинарные данные.
	SecretTypeBinary SecretType = "binary"
	// SecretTypeCard — данные банковской карты.
	SecretTypeCard SecretType = "card"
)

// IsValid сообщает, является ли тип секрета известным системе.
func (t SecretType) IsValid() bool {
	switch t {
	case SecretTypeCredentials, SecretTypeText, SecretTypeBinary, SecretTypeCard:
		return true
	default:
		return false
	}
}

// Secret — единица приватных данных пользователя.
//
// Поле Data содержит зашифрованные на клиенте данные; сервер не имеет доступа
// к открытому тексту. Version увеличивается при каждом изменении и используется
// для оптимистичной блокировки при синхронизации. DeletedAt реализует мягкое
// удаление: запись остаётся в базе, чтобы другие клиенты узнали об удалении.
type Secret struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Type      SecretType `json:"type"`
	Name      string     `json:"name"`
	Metadata  string     `json:"metadata"`
	Data      []byte     `json:"data"`
	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
