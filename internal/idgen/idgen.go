// Package idgen содержит абстракцию генерации идентификаторов.
//
// Идентификаторы генерируются в Go, а не в базе данных: значение нужно сразу
// после создания сущности — чтобы вернуть его клиенту и залогировать до
// коммита транзакции. Интерфейс Generator позволяет подменять генератор в
// тестах на предсказуемый.
package idgen

import "github.com/google/uuid"

// Generator генерирует уникальные идентификаторы.
type Generator interface {
	// Generate возвращает новый уникальный идентификатор.
	Generate() string
}

// UUIDGenerator — реализация Generator на основе UUID v4.
type UUIDGenerator struct{}

// NewUUIDGenerator создаёт генератор UUID v4.
func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{}
}

// Generate возвращает новый UUID v4 в строковом виде.
//
// В штатном режиме ошибок не возвращает: uuid.New паникует только при сбое
// криптографического генератора ОС, что является критической ошибкой среды.
func (g *UUIDGenerator) Generate() string {
	return uuid.New().String()
}
