package storage

import (
	"fmt"
	"path/filepath"
	"testing"
)

// benchSizes — размеры базы для замеров: небольшая личная база и заметно
// большая, чтобы увидеть вклад количества записей.
var benchSizes = []int{100, 1000}

// openBenchStore создаёт временную базу с n записями.
//
// Запись в BoltDB сопровождается fsync, поэтому наполнение вынесено за цикл
// замера: бенчмарк измеряет только чтение.
func openBenchStore(b *testing.B, n int) *SecretStore {
	b.Helper()

	path := filepath.Join(b.TempDir(), "secrets.db")
	store, err := OpenPath(path)
	if err != nil {
		b.Fatalf("OpenPath вернул ошибку: %v", err)
	}
	b.Cleanup(func() { _ = store.Close() })

	for i := 0; i < n; i++ {
		if err := store.Save(testSecret(fmt.Sprintf("id-%d", i))); err != nil {
			b.Fatalf("Save вернул ошибку: %v", err)
		}
	}
	return store
}

// BenchmarkList измеряет полный обход базы без фильтрации.
func BenchmarkList(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			store := openBenchStore(b, n)

			b.ReportAllocs()
			for b.Loop() {
				if _, err := store.List(); err != nil {
					b.Fatalf("List вернул ошибку: %v", err)
				}
			}
		})
	}
}

// BenchmarkSearch измеряет поиск с фильтрацией на ленивом обходе.
func BenchmarkSearch(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			store := openBenchStore(b, n)

			b.ReportAllocs()
			for b.Loop() {
				if _, err := store.Search("id-5"); err != nil {
					b.Fatalf("Search вернул ошибку: %v", err)
				}
			}
		})
	}
}
