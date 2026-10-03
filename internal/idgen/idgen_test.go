package idgen

import (
	"testing"

	"github.com/google/uuid"
)

func TestUUIDGeneratorGenerate(t *testing.T) {
	gen := NewUUIDGenerator()

	first := gen.Generate()
	parsed, err := uuid.Parse(first)
	if err != nil {
		t.Fatalf("Generate вернул невалидный UUID %q: %v", first, err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("ожидалась версия UUID 4, получена %d", parsed.Version())
	}

	second := gen.Generate()
	if first == second {
		t.Fatal("два вызова Generate вернули одинаковый UUID")
	}
	if _, err := uuid.Parse(second); err != nil {
		t.Fatalf("второй UUID невалиден: %v", err)
	}
}

// fakeGenerator возвращает заранее заданные идентификаторы по порядку.
// Используется в тестах других пакетов для детерминированных значений.
type fakeGenerator struct {
	ids []string
	i   int
}

func (f *fakeGenerator) Generate() string {
	id := f.ids[f.i]
	f.i++
	return id
}

func TestGeneratorInterface(t *testing.T) {
	var gen Generator = &fakeGenerator{ids: []string{"first", "second"}}

	if got := gen.Generate(); got != "first" {
		t.Fatalf("Generate = %q, ожидалось first", got)
	}
	if got := gen.Generate(); got != "second" {
		t.Fatalf("Generate = %q, ожидалось second", got)
	}
}
