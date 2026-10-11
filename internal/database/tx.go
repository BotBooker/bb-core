package database

import (
	"context"
	"database/sql"
	"fmt"

	trm "github.com/avito-tech/go-transaction-manager/trm/v2"
	trmcontext "github.com/avito-tech/go-transaction-manager/trm/v2/context"
	"github.com/avito-tech/go-transaction-manager/trm/v2/drivers"
)

// TxFactory создаёт trm.TrFactory поверх пула database/sql.
//
// Опции транзакции не передаются: менеджер начинает транзакцию с настройками
// драйвера по умолчанию (уровни изоляции задаются на уровне подключения/DSN).
// Вложенные транзакции (PropagationNested) не создают SAVEPOINT: manager
// выполняет запросы в текущей транзакции, так как transaction не реализует
// trm.NestedTrFactory.
func TxFactory(pool SQLDB) trm.TrFactory {
	return func(ctx context.Context, _ trm.Settings) (context.Context, trm.Transaction, error) {
		tx, err := pool.BeginTx(ctx, nil)
		if err != nil {
			return ctx, nil, fmt.Errorf("%w: %w", trm.ErrBegin, err)
		}

		return ctx, newTransaction(tx), nil
	}
}

// GetTx возвращает транзакцию из контекста, открытую через TrManager.
// Второе значение — признак того, что транзакция найдена.
//
// Репозиториям внутри TrManager().Do() нужно работать именно с этой транзакцией,
// иначе запрос уйдёт в пул и не попадёт в текущую транзакцию.
func GetTx(ctx context.Context) (*sql.Tx, bool) {
	tr := trmcontext.DefaultManager.Default(ctx)
	if tr == nil {
		return nil, false
	}

	tx, ok := tr.Transaction().(*sql.Tx)

	return tx, ok
}

// transaction — trm.Transaction поверх транзакции database/sql.
type transaction struct {
	tx       *sql.Tx
	isClosed *drivers.IsClosed
}

func newTransaction(tx *sql.Tx) *transaction {
	return &transaction{
		tx:       tx,
		isClosed: drivers.NewIsClosed(),
	}
}

// Transaction возвращает реальную транзакцию database/sql.
func (t *transaction) Transaction() any {
	return t.tx
}

// Commit фиксирует транзакцию.
func (t *transaction) Commit(_ context.Context) error {
	defer t.isClosed.Close()

	return t.tx.Commit()
}

// Rollback откатывает транзакцию.
func (t *transaction) Rollback(_ context.Context) error {
	defer t.isClosed.Close()

	return t.tx.Rollback()
}

// IsActive сообщает, что транзакция начата и ещё не завершена.
func (t *transaction) IsActive() bool {
	return t.isClosed.IsActive()
}

// Closed возвращает канал, который закрывается после Commit или Rollback.
func (t *transaction) Closed() <-chan struct{} {
	return t.isClosed.Closed()
}
