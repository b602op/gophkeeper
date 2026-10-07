package crypto

import (
	"bytes"
	"testing"
)

// benchSize описывает один размер полезной нагрузки для бенчмарков.
type benchSize struct {
	name string
	size int
}

// benchSizes — типовые объёмы: мелкий секрет, заметка, крупный файл.
func benchSizes() []benchSize {
	return []benchSize{
		{"16B", 16},
		{"1KiB", 1 << 10},
		{"64KiB", 64 << 10},
	}
}

// benchmarkKey выводит ключ один раз на бенчмарк: Argon2id намеренно дорог, и
// его стоимость не должна попадать в замеры Encrypt/Decrypt.
func benchmarkKey(b *testing.B) []byte {
	b.Helper()

	salt, err := GenerateSalt()
	if err != nil {
		b.Fatalf("GenerateSalt вернул ошибку: %v", err)
	}
	return DeriveKey("мастер-пароль", salt)
}

// BenchmarkDeriveKey измеряет стоимость вывода ключа Argon2id. Это самая
// ресурсоёмкая операция пакета, выполняемая при входе пользователя.
func BenchmarkDeriveKey(b *testing.B) {
	salt, err := GenerateSalt()
	if err != nil {
		b.Fatalf("GenerateSalt вернул ошибку: %v", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = DeriveKey("мастер-пароль", salt)
	}
}

// BenchmarkEncrypt измеряет шифрование на разных объёмах данных.
func BenchmarkEncrypt(b *testing.B) {
	key := benchmarkKey(b)

	for _, size := range benchSizes() {
		b.Run(size.name, func(b *testing.B) {
			plaintext := bytes.Repeat([]byte("a"), size.size)

			b.ReportAllocs()
			b.SetBytes(int64(size.size))
			for b.Loop() {
				if _, err := Encrypt(key, plaintext); err != nil {
					b.Fatalf("Encrypt вернул ошибку: %v", err)
				}
			}
		})
	}
}

// BenchmarkDecrypt измеряет расшифровку на разных объёмах данных. Шифротекст
// готовится до цикла: Encrypt генерирует случайный nonce и не должен входить в
// замер расшифровки.
func BenchmarkDecrypt(b *testing.B) {
	key := benchmarkKey(b)

	for _, size := range benchSizes() {
		b.Run(size.name, func(b *testing.B) {
			plaintext := bytes.Repeat([]byte("a"), size.size)
			encoded, err := Encrypt(key, plaintext)
			if err != nil {
				b.Fatalf("Encrypt вернул ошибку: %v", err)
			}

			b.ReportAllocs()
			b.SetBytes(int64(size.size))
			for b.Loop() {
				if _, err := Decrypt(key, encoded); err != nil {
					b.Fatalf("Decrypt вернул ошибку: %v", err)
				}
			}
		})
	}
}
