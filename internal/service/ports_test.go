package service

import (
	"context"

	"github.com/botbooker/bb-core/pkg/password"
)

// testHasher — настоящая реализация порта domain.PasswordHasher.
//
// Сервису важен именно результат проверки: тесты входа используют хеш прежнего
// формата (PBKDF2) и проверяют его прозрачный пересчёт, а подделка скрыла бы
// расхождение порта с реализацией. Сам Argon2id проверяется в pkg/password.
var testHasher = password.NewHasher()

// testUnitOfWork — UnitOfWork без транзакции: репозитории в тестах сервисов
// заменены моками, поэтому Do просто выполняет функцию. Откат на ошибке
// проверяется интеграционными тестами repository, где есть настоящая база.
type testUnitOfWork struct{}

// Do выполняет fn в текущем контексте.
func (testUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
