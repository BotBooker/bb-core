// Package sqlite реализует подключение к SQLite через database/sql.
package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"

	// Драйвер "sqlite": именованный импорт, потому что нужен NewConnector —
	// через него включается проверка внешних ключей. Регистрацию драйвера в
	// database/sql делает init() пакета, поэтому имя "sqlite" тоже доступно.
	sqlitedriver "modernc.org/sqlite"
)

const (
	// DriverName — имя драйвера SQLite, регистрируемого modernc.org/sqlite.
	// По совместимости с database/sql это имя, а не диалект из конфигурации.
	DriverName = "sqlite"
	// defaultMaxOpenConns — число одновременных соединений по умолчанию.
	//
	// SQLite допускает только одного писателя, поэтому пул ограничен одним
	// соединением: так не возникает ошибок "database is locked", а база
	// в памяти (:memory:) не распадается на отдельные копии для каждого соединения.
	// Для файловой базы в режиме WAL значение можно увеличить через MAX_DB_CONNECT.
	defaultMaxOpenConns = 1
	// pingTimeout — таймаут проверки доступности базы данных при старте.
	pingTimeout = 5 * time.Second
)

// StatementBuilder — построитель SQL-запросов с плейсхолдерами SQLite (?).
var StatementBuilder = sq.StatementBuilder.PlaceholderFormat(sq.Question)

// poolFactory создаёт пул соединений по конфигурации.
// Передаётся аргументом, а не вызывается напрямую, чтобы unit-тесты
// могли подставить фейковый пул без работы с файловой системой.
type poolFactory func(ctx context.Context, cfg config.DatabaseConfig) (database.SQLDB, error)

// defaultPoolFactory создаёт реальный пул соединений database/sql.
func defaultPoolFactory(_ context.Context, cfg config.DatabaseConfig) (database.SQLDB, error) {
	connector, err := sqlitedriver.NewConnector(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Пул создаётся через коннектор, а не через sql.Open: только так каждое
	// соединение получает PRAGMA foreign_keys (см. foreignKeysConnector).
	pool := sql.OpenDB(foreignKeysConnector{inner: connector})

	// Пулы ленивые: sql.Open не устанавливает соединение и не читает файл,
	// поэтому настройки применяем сразу, а доступность проверяем в connect.
	maxOpenConns := defaultMaxOpenConns
	if cfg.MaxDBConnect > 0 {
		maxOpenConns = int(cfg.MaxDBConnect)
	}

	// MaxIdleConns равен MaxOpenConns, иначе единственное соединение закроется
	// и база в памяти потеряет данные.
	pool.SetMaxOpenConns(maxOpenConns)
	pool.SetMaxIdleConns(maxOpenConns)

	return pool, nil
}

// foreignKeysPragma — прагма, включающая проверку внешних ключей в SQLite.
const foreignKeysPragma = "PRAGMA foreign_keys = ON"

// foreignKeysConnector включает проверку внешних ключей на каждом соединении.
//
// SQLite по умолчанию внешние ключи не проверяет (PRAGMA foreign_keys = OFF),
// поэтому ON DELETE CASCADE из миграций молча не работал бы: удаление
// пользователя оставляло бы его токены и учётные данные. Параметр DSN
// (_pragma=foreign_keys(1)) решает задачу только в URI-форме — в обычном пути
// ("/tmp/database.db", ":memory:") драйвер его не разбирает. Обёртка над
// коннектором снимает эту разницу: прагма выполняется при открытии каждого
// соединения и не зависит от вида DSN.
type foreignKeysConnector struct {
	inner driver.Connector
}

// Connect открывает соединение и включает на нём проверку внешних ключей.
// Если прагма не применилась, соединение закрывается: без неё работа с базой
// нарушила бы ссылочную целостность, а это хуже явной ошибки старта.
func (c foreignKeysConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}

	if err := enableForeignKeys(conn); err != nil {
		_ = conn.Close()

		return nil, err
	}

	return conn, nil
}

// Driver возвращает драйвер внутреннего коннектора.
func (c foreignKeysConnector) Driver() driver.Driver {
	return c.inner.Driver()
}

// enableForeignKeys включает проверку внешних ключей на одном соединении.
func enableForeignKeys(conn driver.Conn) error {
	statement, err := conn.Prepare(foreignKeysPragma)
	if err != nil {
		return fmt.Errorf("prepare %q: %w", foreignKeysPragma, err)
	}

	defer func() {
		// Ошибку закрытия вернуть некуда: закрывает соединение вызывающий.
		_ = statement.Close()
	}()

	if _, err := statement.Exec(nil); err != nil {
		return fmt.Errorf("exec %q: %w", foreignKeysPragma, err)
	}

	return nil
}

// New создаёт подключение к SQLite: открывает пул соединений,
// проверяет доступность базы и создаёт менеджер транзакций.
//
// DSN — путь к файлу (":memory:" для базы в памяти) или URI "file:...".
// Префикс "sqlite:"/"sqlite3:" тоже допускается: он отбрасывается (см. normalizeDSN).
func New(ctx context.Context, cfg config.DatabaseConfig) (database.DB, error) {
	return newWithFactory(ctx, cfg, defaultPoolFactory)
}

// newWithFactory создаёт подключение с подменяемой фабрикой пула соединений.
func newWithFactory(ctx context.Context, cfg config.DatabaseConfig, factory poolFactory) (database.DB, error) {
	pool, err := connect(ctx, cfg, factory)
	if err != nil {
		return nil, err
	}

	// database.New закрывает пул, если менеджер транзакций создать не удалось.
	db, err := database.New(pool)
	if err != nil {
		return nil, fmt.Errorf("create database connection: %w", err)
	}

	slog.Debug("connected to the database", "driver", DriverName)

	return db, nil
}

// connect проверяет конфигурацию, создаёт пул соединений и проверяет соединение с базой.
func connect(ctx context.Context, cfg config.DatabaseConfig, factory poolFactory) (database.SQLDB, error) {
	// Нормализуем DSN до вызова фабрики: и проверка, и пул должны видеть одно значение.
	cfg.DSN = normalizeDSN(cfg.DSN)
	if cfg.DSN == "" {
		return nil, database.ErrEmptyDSN
	}

	pool, err := factory(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	// Ping открывает первое соединение: здесь выясняется,
	// существует ли файл базы и доступен ли он для записи.
	if err := pool.PingContext(pingCtx); err != nil {
		_ = pool.Close()

		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return pool, nil
}

// normalizeDSN приводит DSN к формату, который понимает драйвер modernc.org/sqlite.
//
// Драйвер принимает путь к файлу (":memory:", "/tmp/database.db") или URI "file:...",
// но не понимает схему в стиле других драйверов: с DSN=sqlite:/tmp/database.db
// он пытается открыть файл с таким именем и падает с "unable to open database file (14)".
// Поэтому префикс "sqlite:"/"sqlite3:" отбрасывается, а остаток передаётся драйверу как есть.
func normalizeDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)

	// Длинные префиксы проверяем первыми, иначе "sqlite://" разрежется как "sqlite:".
	for _, prefix := range []string{"sqlite3://", "sqlite://", "sqlite3:", "sqlite:"} {
		if rest, found := strings.CutPrefix(dsn, prefix); found {
			return strings.TrimSpace(rest)
		}
	}

	return dsn
}
