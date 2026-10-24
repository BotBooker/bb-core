package domain

import "context"

// UnitOfWork — порт транзакционной границы use case (spec §7).
//
// Реализация — адаптер над менеджером транзакций (internal/database):
// репозитории получают открытую транзакцию из контекста и выполняют в ней свои
// запросы. Один вызов Do — одна транзакция: ошибка откатывает все изменения,
// nil фиксирует их.
//
// Вложенные Do выполняются в уже открытой транзакции (реализация trm/v2 не
// создаёт SAVEPOINT): use case не должен рассчитывать на частичный откат.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}
