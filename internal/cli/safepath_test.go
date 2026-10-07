package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/b602op/gophkeeper/internal/crypto"
)

// TestOpenRestrictedFile покрывает хелпер ограничения путей: относительные пути
// не могут выйти за пределы корня, абсолютные открываются напрямую.
func TestOpenRestrictedFile(t *testing.T) {
	t.Run("относительный путь внутри корня", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		if err := os.WriteFile("data.bin", []byte{0x01}, 0o600); err != nil {
			t.Fatalf("WriteFile вернул ошибку: %v", err)
		}

		f, err := openRestrictedFile(dir, "data.bin")
		if err != nil {
			t.Fatalf("openRestrictedFile вернул ошибку: %v", err)
		}
		defer func() { _ = f.Close() }()

		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll вернул ошибку: %v", err)
		}
		if len(data) != 1 {
			t.Fatalf("прочитано %d байт, ожидался 1", len(data))
		}
	})

	t.Run("выход за корень отклоняется", func(t *testing.T) {
		_, err := openRestrictedFile(t.TempDir(), "../../etc/passwd")
		if err == nil {
			t.Fatal("ожидалась ошибка для пути за пределами корня")
		}
		if !strings.Contains(err.Error(), "путь выходит за пределы разрешённой директории") {
			t.Fatalf("неожиданное сообщение об ошибке: %v", err)
		}
	})

	t.Run("абсолютный путь открывается", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "abs.bin")
		if err := os.WriteFile(path, []byte{0x02}, 0o600); err != nil {
			t.Fatalf("WriteFile вернул ошибку: %v", err)
		}

		f, err := openRestrictedFile(t.TempDir(), path)
		if err != nil {
			t.Fatalf("openRestrictedFile вернул ошибку: %v", err)
		}
		defer func() { _ = f.Close() }()
	})

	t.Run("отсутствующий локальный файл — обычная ошибка", func(t *testing.T) {
		dir := t.TempDir()
		_, err := openRestrictedFile(dir, "missing.bin")
		if err == nil {
			t.Fatal("ожидалась ошибка для отсутствующего файла")
		}
		if strings.Contains(err.Error(), "путь выходит за пределы") {
			t.Fatalf("отсутствие файла ошибочно трактовано как выход за корень: %v", err)
		}
	})
}

// TestAddBinaryTraversalRejected проверяет, что CLI-команда add не позволяет
// прочитать файл за пределами текущей директории через относительный путь.
func TestAddBinaryTraversalRejected(t *testing.T) {
	env := newTestEnv(t, "")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	err := env.execute("add", "Файл", "--type", "binary", "--file", "../../etc/passwd")
	if err == nil {
		t.Fatal("ожидалась ошибка для пути за пределами директории")
	}
	if !strings.Contains(err.Error(), "путь выходит за пределы разрешённой директории") {
		t.Fatalf("неожиданное сообщение об ошибке: %v", err)
	}
}

// TestAddBinaryTraversalInteractive проверяет тот же запрет при вводе пути через
// интерактивный запрос (без флага --file).
func TestAddBinaryTraversalInteractive(t *testing.T) {
	env := newTestEnv(t, "../../etc/passwd\n")
	env.authenticate(t, "user-1")
	salt, _ := crypto.GenerateSalt()
	env.sess.SetSalt(salt)
	env.sess.SetMasterKey(crypto.DeriveKey("master-pass", salt))

	err := env.execute("add", "Файл", "--type", "binary")
	if err == nil {
		t.Fatal("ожидалась ошибка для пути за пределами директории")
	}
	if !strings.Contains(err.Error(), "путь выходит за пределы разрешённой директории") {
		t.Fatalf("неожиданное сообщение об ошибке: %v", err)
	}
}
