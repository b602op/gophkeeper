package main

import (
	"log"
	"os"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1) // want "использование os.Exit в main запрещено"
	}
	log.Println("готово")
}

func run() error {
	return nil
}
