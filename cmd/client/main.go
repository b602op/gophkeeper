// Command client — CLI-клиент GophKeeper.
//
// Все команды реализованы в пакете internal/cli на базе cobra: регистрация и
// вход, работа с секретами (add/get/list/search/update/delete), синхронизация и
// вывод версии. Точка входа лишь собирает корневую команду и обрабатывает
// ошибку.
package main

import (
	"log"
	"os"

	"github.com/b602op/gophkeeper/internal/cli"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("клиент остановлен с ошибкой: %v", err)
	}
}

// run выполняет команду CLI и возвращает ошибку вместо прямого выхода.
func run(args []string) error {
	root := cli.NewRootCommand()
	root.SetArgs(args)
	return root.Execute()
}
