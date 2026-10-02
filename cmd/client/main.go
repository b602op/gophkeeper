// Command client — CLI-клиент GophKeeper.
//
// На текущем этапе реализована только команда version; полноценные команды
// работы с секретами добавляются на следующем этапе.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/b602op/gophkeeper/internal/buildinfo"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("клиент остановлен с ошибкой: %v", err)
	}
}

// run выполняет команду CLI и возвращает ошибку вместо прямого выхода.
func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "version":
		fmt.Println(buildinfo.String())
	case "help", "-h", "--help":
		printUsage()
	default:
		return fmt.Errorf("неизвестная команда %q", args[0])
	}
	return nil
}

// printUsage печатает краткую справку по командам клиента.
func printUsage() {
	fmt.Println("GophKeeper — менеджер паролей")
	fmt.Println()
	fmt.Println("Использование: gophkeeper <команда>")
	fmt.Println()
	fmt.Println("Команды:")
	fmt.Println("  version   показать версию и дату сборки")
	fmt.Println("  help      показать эту справку")
}
