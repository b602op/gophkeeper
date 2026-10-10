package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// openRestrictedFile открывает пользовательский файл, защищаясь от выхода за
// пределы разрешённой директории (path traversal) для относительных путей.
//
// Относительный путь ограничивается каталогом rootDir через os.OpenRoot: этот
// механизм не позволит выйти наружу ни через "../", ни через симлинки, и не
// подвержен гонке TOCTOU, в отличие от ручной проверки filepath.Clean и
// strings.HasPrefix.
//
// Абсолютный путь открывается напрямую: это явный выбор пользователя, а не
// path traversal, поэтому ограничивать его корнем нельзя — иначе сломались бы
// привычные сценарии вроде "gophkeeper add --file /home/me/secret.bin".
//
// Предварительная проверка filepath.IsLocal нужна только для того, чтобы
// отличить выход за пределы директории от обычных ошибок открытия (нет файла,
// нет прав) и вернуть понятное сообщение.
func openRestrictedFile(rootDir, userPath string) (*os.File, error) {
	if filepath.IsAbs(userPath) {
		f, err := os.Open(userPath)
		if err != nil {
			return nil, fmt.Errorf("не удалось открыть файл %q: %w", userPath, err)
		}
		return f, nil
	}

	if !filepath.IsLocal(userPath) {
		return nil, fmt.Errorf("не удалось открыть файл %q: путь выходит за пределы разрешённой директории", userPath)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть корневую директорию %q: %w", rootDir, err)
	}
	// root.Close освобождает дескриптор корневого каталога; уже открытый файл
	// имеет собственный дескриптор и остаётся доступным для чтения.
	defer func() { _ = root.Close() }()

	f, err := root.Open(userPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл %q: %w", userPath, err)
	}
	return f, nil
}
