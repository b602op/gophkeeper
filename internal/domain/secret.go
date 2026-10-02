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

// Valid сообщает, является ли тип секрета известным системе.
func (t SecretType) Valid() bool {
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
// для синхронизации. DeletedAt реализует мягкое удаление.
type Secret struct {
	ID        string
	UserID    string
	Type      SecretType
	Name      string
	Metadata  string
	Data      []byte
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}
