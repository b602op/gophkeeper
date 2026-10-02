package main

import "log"

func main() {
	if err := run(); err != nil {
		log.Fatalf("ошибка: %v", err)
	}
}

func run() error {
	return nil
}
