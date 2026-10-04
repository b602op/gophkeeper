package crypto

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// testKey выводит ключ один раз: Argon2id намеренно ресурсоёмкий, поэтому в
// тестах избегаем лишних вызовов.
func testKey(t *testing.T) (key, salt []byte) {
	t.Helper()
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt вернул ошибку: %v", err)
	}
	return DeriveKey("мастер-пароль", salt), salt
}

func TestDeriveKeyDeterministic(t *testing.T) {
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt вернул ошибку: %v", err)
	}

	first := DeriveKey("пароль", salt)
	second := DeriveKey("пароль", salt)

	if len(first) != argonKeyLen {
		t.Fatalf("длина ключа = %d, ожидалось %d", len(first), argonKeyLen)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("одинаковые пароль и соль дали разные ключи")
	}
}

func TestDeriveKeyDependsOnSaltAndPassword(t *testing.T) {
	saltA, _ := GenerateSalt()
	saltB, _ := GenerateSalt()

	if bytes.Equal(DeriveKey("пароль", saltA), DeriveKey("пароль", saltB)) {
		t.Fatal("разные соли дали одинаковый ключ")
	}
	if bytes.Equal(DeriveKey("пароль-1", saltA), DeriveKey("пароль-2", saltA)) {
		t.Fatal("разные пароли дали одинаковый ключ")
	}
}

func TestGenerateSalt(t *testing.T) {
	first, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt вернул ошибку: %v", err)
	}
	if len(first) != saltLen {
		t.Fatalf("длина соли = %d, ожидалось %d", len(first), saltLen)
	}

	second, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt вернул ошибку: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("две соли оказались одинаковыми")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, _ := testKey(t)

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"текст", []byte("секретные данные")},
		{"пустые данные", []byte{}},
		{"бинарные данные", []byte{0x00, 0x01, 0xFF, 0xFE, 0x10}},
		{"длинные данные", bytes.Repeat([]byte("x"), 4096)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := Encrypt(key, tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt вернул ошибку: %v", err)
			}
			if !strings.HasPrefix(encoded, cipherVersion+separator) {
				t.Fatalf("шифротекст не начинается с %q: %q", cipherVersion, encoded)
			}

			got, err := Decrypt(key, encoded)
			if err != nil {
				t.Fatalf("Decrypt вернул ошибку: %v", err)
			}
			if !bytes.Equal(got, tt.plaintext) {
				t.Fatalf("Decrypt вернул %q, ожидалось %q", got, tt.plaintext)
			}
		})
	}
}

func TestEncryptUsesRandomNonce(t *testing.T) {
	key, _ := testKey(t)

	first, err := Encrypt(key, []byte("данные"))
	if err != nil {
		t.Fatalf("Encrypt вернул ошибку: %v", err)
	}
	second, err := Encrypt(key, []byte("данные"))
	if err != nil {
		t.Fatalf("Encrypt вернул ошибку: %v", err)
	}
	if first == second {
		t.Fatal("два шифрования дали одинаковый результат: nonce не случаен")
	}
}

func TestDecryptInvalidFormat(t *testing.T) {
	key, _ := testKey(t)

	tests := []struct {
		name    string
		encoded string
	}{
		{"пустая строка", ""},
		{"одна часть", "v1"},
		{"две части", "v1:abcd"},
		{"неизвестная версия", "v2:YWJjZGVmZ2hpamts:YWJj"},
		{"битый base64 nonce", "v1:!!!:YWJj"},
		{"короткий nonce", "v1:YWJj:YWJj"},
		{"битый base64 шифротекста", "v1:YWJjZGVmZ2hpamts:!!!"},
		{"пустой шифротекст", "v1:YWJjZGVmZ2hpamts:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decrypt(key, tt.encoded)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if !errors.Is(err, ErrInvalidFormat) {
				t.Fatalf("ошибка %v не оборачивает ErrInvalidFormat", err)
			}
		})
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key, _ := testKey(t)
	otherKey, _ := testKey(t)

	encoded, err := Encrypt(key, []byte("данные"))
	if err != nil {
		t.Fatalf("Encrypt вернул ошибку: %v", err)
	}

	_, err = Decrypt(otherKey, encoded)
	if err == nil {
		t.Fatal("ожидалась ошибка расшифровки неверным ключом")
	}
	if !errors.Is(err, ErrDecrypt) {
		t.Fatalf("ошибка %v не оборачивает ErrDecrypt", err)
	}
}

func TestDecryptInvalidKeyLength(t *testing.T) {
	encoded, err := Encrypt(make([]byte, 32), []byte("данные"))
	if err != nil {
		t.Fatalf("Encrypt вернул ошибку: %v", err)
	}

	if _, err := Decrypt([]byte("короткий"), encoded); err == nil {
		t.Fatal("ожидалась ошибка при неверной длине ключа")
	}
}

func TestPackUnpackPayload(t *testing.T) {
	key, salt := testKey(t)

	encoded, err := Encrypt(key, []byte("данные"))
	if err != nil {
		t.Fatalf("Encrypt вернул ошибку: %v", err)
	}

	payload, err := PackPayload(salt, encoded)
	if err != nil {
		t.Fatalf("PackPayload вернул ошибку: %v", err)
	}
	if len(payload) != PayloadSaltLen+len(encoded) {
		t.Fatalf("длина payload = %d, ожидалось %d", len(payload), PayloadSaltLen+len(encoded))
	}

	gotSalt, gotEncoded, err := UnpackPayload(payload)
	if err != nil {
		t.Fatalf("UnpackPayload вернул ошибку: %v", err)
	}
	if !bytes.Equal(gotSalt, salt) {
		t.Fatal("извлечённая соль не совпадает с исходной")
	}
	if gotEncoded != encoded {
		t.Fatalf("извлечённый шифротекст = %q, ожидался %q", gotEncoded, encoded)
	}

	// Полный цикл: ключ выводится из извлечённой соли и расшифровывает данные.
	recovered := DeriveKey("мастер-пароль", gotSalt)
	plaintext, err := Decrypt(recovered, gotEncoded)
	if err != nil {
		t.Fatalf("Decrypt вернул ошибку: %v", err)
	}
	if string(plaintext) != "данные" {
		t.Fatalf("Decrypt вернул %q, ожидалось %q", plaintext, "данные")
	}
}

func TestPackPayloadInvalidSalt(t *testing.T) {
	if _, err := PackPayload([]byte("short"), "v1:a:b"); err == nil {
		t.Fatal("ожидалась ошибка при неверной длине соли")
	}
}

func TestUnpackPayloadTooShort(t *testing.T) {
	if _, _, err := UnpackPayload(make([]byte, PayloadSaltLen)); err == nil {
		t.Fatal("ожидалась ошибка при слишком коротких данных")
	}
	if !errors.Is(mustUnpackErr(make([]byte, 3)), ErrInvalidFormat) {
		t.Fatal("ошибка не оборачивает ErrInvalidFormat")
	}
}

// mustUnpackErr возвращает ошибку UnpackPayload для проверки через errors.Is.
func mustUnpackErr(payload []byte) error {
	_, _, err := UnpackPayload(payload)
	return err
}
