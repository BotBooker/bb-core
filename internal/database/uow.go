// Package database содержит адаптер Unit of Work над менеджером транзакций.
package database

import (
	"context"
	"errors"
	"fmt"

	trm "github.com/avito-tech/go-transaction-manager/trm/v2"
)

// UnitOfWork — адаптер trm.Manager под порт domain.UnitOfWork.
//
// Реализация не импортирует application-слой: порт объявлен в домене, а
// адаптер лишь переводит вызов Do в менеджер транзакций. Транзакция попадает в
// контекст (trmcontext.DefaultManager), поэтому репозитории, вызванные внутри
// функции, работают в этой же транзакции через database.GetTx.
type UnitOfWork struct {
	manager trm.Manager
}

// NewUnitOfWork создаёт адаптер над менеджером транзакций.
func NewUnitOfWork(manager trm.Manager) *UnitOfWork {
	return &UnitOfWork{manager: manager}
}

// Do выполняет fn в транзакции: ошибка откатывает изменения, nil фиксирует их.
//
// Вложенный вызов внутри уже открытой транзакции выполняется в ней же
// (менеджер не создаёт SAVEPOINT): use case не должен рассчитывать на
// частичный откат.
func (u *UnitOfWork) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if u == nil || u.manager == nil {
		return errors.New("unit of work: transaction manager is not configured")
	}

	if err := u.manager.Do(ctx, fn); err != nil {
		return fmt.Errorf("unit of work: %w", err)
	}

	return nil
}
