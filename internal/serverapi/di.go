package serverapi

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/botbooker/bb-core/internal/cache"
	"github.com/botbooker/bb-core/internal/closer"
	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/database/postgres"
	"github.com/botbooker/bb-core/internal/database/sqlite"
	"github.com/botbooker/bb-core/internal/events"
	"github.com/botbooker/bb-core/internal/jwt"
	"github.com/botbooker/bb-core/internal/repository"
	"github.com/botbooker/bb-core/internal/service"
	user_v1 "github.com/botbooker/bb-core/pkg/proto/user/v1"
)

// newDatabase подключается к базе данных.
//
// Переменная, а не прямой вызов: unit-тесты DI подменяют подключение,
// чтобы не ходить в настоящую базу данных.
var newDatabase = newDatabaseByDSN

// migrateDatabase применяет миграции к подключению.
//
// Переменная, а не прямой вызов: unit-тесты DI подменяют её так же, как
// подключение, чтобы не ходить в настоящую базу данных.
var migrateDatabase = applyMigrations

// applyMigrations применяет миграции при старте, если это разрешено настройкой.
// Диалект берётся из того же DSN, по которому выбирался драйвер: отдельного
// поля с диалектом нет.
func applyMigrations(ctx context.Context, db database.DB) error {
	cfg := config.DBConfig()
	if !cfg.MigrateOnStart {
		slog.Debug("migrations on start are disabled")

		return nil
	}

	dialect, err := database.DialectFromDSN(cfg.DSN)
	if err != nil {
		return err
	}

	return database.Migrate(ctx, db, dialect)
}

// newPostgresDB и newSQLiteDB — конструкторы драйверов.
// Подменяются в тестах, чтобы проверить выбор драйвера по DSN.
var (
	newPostgresDB = postgres.New
	newSQLiteDB   = sqlite.New
)

// newDatabaseByDSN выбирает драйвер по строке подключения.
// Отдельного поля с диалектом нет: он определяется из DSN, поэтому
// нельзя случайно подключиться не тем драйвером.
// Само подключение живёт в пакетах-драйверах: у них нет общей точки входа,
// потому что драйверы зависят от database.DB, а не наоборот.
func newDatabaseByDSN(ctx context.Context) (database.DB, error) {
	cfg := config.DBConfig()

	dialect, err := database.DialectFromDSN(cfg.DSN)
	if err != nil {
		return nil, err
	}

	switch dialect {
	case database.DialectPostgres:
		return newPostgresDB(ctx, *cfg)
	case database.DialectSQLite:
		return newSQLiteDB(ctx, *cfg)
	default:
		// Недостижимо: DialectFromDSN возвращает только константы выше.
		return nil, fmt.Errorf("%w: %q", database.ErrUnsupportedDialect, dialect)
	}
}

// diContainer — контейнер зависимостей с ленивой инициализацией.
//
// Это тот же контейнер из видео про DI, но при создании каждого ресурса
// с методом Close() мы сразу регистрируем его в глобальном closer.
// closer.Add() вызывается прямо в геттере — одна строка, и ресурс
// автоматически закроется при graceful shutdown в правильном LIFO-порядке.
//
// БД подключается сразу в newDIContainer: без неё приложение работать не может,
// поэтому ошибка возвращается вызывающему коду, а не приводится к os.Exit.
// БД создаётся первой — значит закроется последней (LIFO): кэш закроется раньше БД.
type diContainer struct {
	// Инфраструктура
	db       database.DB
	cache    cache.Cache
	eventBus events.EventBus
	// tokenCodec подписывает и проверяет токены доступа. Создаётся вместе с
	// контейнером: секрет подписи берётся из конфигурации, и ошибку его
	// подготовки (crypto/rand) лучше увидеть на старте, а не при первом входе.
	tokenCodec service.AuthTokenCodec
	// dialect — диалект подключённой базы: репозитории подставляют по нему
	// плейсхолдеры и кодируют время.
	dialect string

	// Репозитории
	userRepo         repository.UserRepo
	tokenRepo        repository.TokenRepo
	credentialRepo   repository.CredentialRepo
	notificationRepo repository.NotificationRepo

	// Сервисы
	authService         service.AuthService
	userService         service.UserService
	notificationService service.NotificationService

	// API
	handler           Handler
	userServiceServer user_v1.UserServiceServer
}

// newDIContainer создаёт контейнер, подключается к базе данных и применяет миграции.
// Ошибка подключения или миграций возвращается сразу: приложению без БД работать нечем.
func newDIContainer(ctx context.Context) (*diContainer, error) {
	db, err := newDatabase(ctx)
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	// Диалект берётся из того же DSN, по которому выбрано подключение:
	// отдельного поля с диалектом нет, поэтому он не может «разойтись» с базой.
	// Ошибка здесь недостижима после успешного подключения, но обрабатывается
	// так же строго: молча работать «не тем» диалектом хуже, чем упасть.
	dialect, err := database.DialectFromDSN(config.DBConfig().DSN)
	if err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("init database: %w", err)
	}

	// Миграции применяются до регистрации closer: если не удалось, пул
	// закрываем сами, иначе соединение висело бы до конца процесса.
	if err := migrateDatabase(ctx, db); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("init database: %w", err)
	}

	// При создании — сразу регистрируем Close() в глобальном closer.
	closer.Add("database", func(_ context.Context) error {
		return db.Close()
	})

	// Подпись токенов настраивается здесь же: секрет из конфигурации, а ошибка
	// его подготовки должна валить старт, а не выясняться при первом входе.
	tokenCodec, err := jwt.NewSigner(config.Auth().JWTSecret, config.Auth().Issuer)
	if err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("init token signer: %w", err)
	}

	return &diContainer{db: db, dialect: dialect, tokenCodec: tokenCodec}, nil
}

// DB возвращает подключение к базе данных, созданное вместе с контейнером.
func (d *diContainer) DB() database.DB {
	return d.db
}

// EventBus возвращает шину событий.
func (d *diContainer) EventBus() events.EventBus {
	if d.eventBus == nil {
		d.eventBus = events.NewEventBus()
	}

	return d.eventBus
}

// Cache возвращает подключение к кэшу.
// При создании — сразу регистрирует Close() в глобальном closer.
// Кэш создаётся после БД — значит закроется раньше БД (LIFO).
func (d *diContainer) Cache() cache.Cache {
	if d.cache == nil {
		c := cache.New(config.AppConfig().RedisAddr)

		closer.Add("cache", func(_ context.Context) error {
			return c.Close()
		})

		d.cache = c
	}

	return d.cache
}

// UserRepo возвращает репозиторий пользователей.
func (d *diContainer) UserRepo() repository.UserRepo {
	if d.userRepo == nil {
		d.userRepo = repository.NewUserRepo(d.DB(), d.dialect)
	}

	return d.userRepo
}

// TokenRepo возвращает репозиторий сессий (токенов доступа).
func (d *diContainer) TokenRepo() repository.TokenRepo {
	if d.tokenRepo == nil {
		d.tokenRepo = repository.NewTokenRepo(d.DB(), d.dialect)
	}

	return d.tokenRepo
}

// CredentialRepo возвращает репозиторий учётных данных (хешей паролей).
func (d *diContainer) CredentialRepo() repository.CredentialRepo {
	if d.credentialRepo == nil {
		d.credentialRepo = repository.NewCredentialRepo(d.DB(), d.dialect)
	}

	return d.credentialRepo
}

// NotificationRepo возвращает репозиторий уведомлений.
func (d *diContainer) NotificationRepo() repository.NotificationRepo {
	if d.notificationRepo == nil {
		d.notificationRepo = repository.NewNotificationRepo(d.DB())
	}

	return d.notificationRepo
}

// AuthService возвращает сервис авторизации.
func (d *diContainer) AuthService() service.AuthService {
	if d.authService == nil {
		d.authService = service.NewAuthService(
			d.UserRepo(),
			d.CredentialRepo(),
			d.TokenRepo(),
			d.tokenCodec,
			d.EventBus(),
			config.Auth().TokenTTL,
		)
	}

	return d.authService
}

// UserService возвращает сервис пользователей.
func (d *diContainer) UserService() service.UserService {
	if d.userService == nil {
		d.userService = service.NewUserService(d.UserRepo(), d.EventBus())
	}

	return d.userService
}

// NotificationService возвращает сервис уведомлений.
func (d *diContainer) NotificationService() service.NotificationService {
	if d.notificationService == nil {
		d.notificationService = service.NewNotificationService(d.NotificationRepo(), d.UserService(), d.EventBus())
	}

	return d.notificationService
}

// Handler возвращает HTTP-хендлер.
func (d *diContainer) Handler() (Handler, error) {
	if d.handler == nil {
		handler, err := NewHandler(d.AuthService(), d.UserServiceServer())
		if err != nil {
			return nil, err
		}

		d.handler = handler
	}

	return d.handler, nil
}

// UserServiceServer возвращает адаптер proto-сервиса пользователей.
// Один экземпляр обслуживает и gRPC, и grpc-gateway: поведение обоих
// транспортов гарантированно одинаково.
func (d *diContainer) UserServiceServer() user_v1.UserServiceServer {
	if d.userServiceServer == nil {
		d.userServiceServer = NewUserServiceServer(d.UserService())
	}

	return d.userServiceServer
}
