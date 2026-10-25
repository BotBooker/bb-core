# Спецификация программирования

| Поле | Значение |
| --- | --- |
| Версия | 1.1 |
| Статус | Approved |
| Область | Backend-сервисы на Go |
| Стек | Go, PostgreSQL (pgx), SQLite (modernc.org/sqlite), HTTP (net/http), gRPC (google.golang.org/grpc), Valkey (Redis) - опционально |

RFC-ключевые: MUST — обязательно, SHOULD — рекомендуется, MAY — допустимо

---

Документ описывает обязательные правила проектирования и реализации backend-сервисов на
Go с применением Domain-Driven Design для стека **PostgreSQL / SQLite + HTTP/gRPC**,
которым следует код репозитория: принципы, работу с логированием и секретами, обработку ошибок,
тестирование, сборку и требования к Kubernetes-манифестам. Это версионируемый источник
правил — `.clinerules/` содержит лишь краткую выжимку со ссылкой сюда, чтобы правила были
видны CI и всем разработчикам.

Отклонение от MUST допускается только с письменным обоснованием в PR.

## 1. Принципы

- **KISS.** Решение должно быть максимально простым из возможных. Абстракции
  вводим, когда они убирают дублирование или скрывают сложность, а не «на
  будущее».
- **SOLID.** Особенно SRP и DIP: пакет отвечает за одну обязанность; зависимости
  передаются через интерфейсы, объявленные на стороне потребителя (например,
  `AuthService` в `internal/servers`).
- **Не присваивать одноразовые значения переменной.** Если значение используется
  только один раз, лучше передать выражение напрямую, а не создавать для него
  отдельную переменную.

### 1.1. Слои и направление зависимостей

| Слой | Ответственность | Разрешённые зависимости |
| --- | --- | --- |
| `domain` | Сущности, value objects, агрегаты, доменные сервисы, интерфейсы репозиториев, доменные ошибки, доменные события | Только stdlib Go |
| `application` | Use cases, оркестрация домена, транзакционная граница, порты | `domain` |
| `infrastructure` | Репозитории (PostgreSQL, SQLite), внешние клиенты, брокеры | `domain`, `application`, драйверы БД |
| `presentation` | HTTP/gRPC-обработчики, DTO, маппинг, middleware | `application` |

**Правило зависимостей:** MUST направляться внутрь. `domain` MUST NOT импортировать
`application`, `infrastructure`, `presentation`, `database/sql`, `pgx`, `google.golang.org/grpc`, `net/http` и любые сторонние библиотеки.

Соответствие слоёв пакетам репозитория:

| Слой | Пакеты | Что внутри |
| --- | --- | --- |
| `domain` | `internal/domain` | порты (репозитории, `UnitOfWork`, `PasswordHasher`), доменные ошибки |
| `domain` | `internal/model` | сущности, value objects, инварианты и валидаторы; только stdlib |
| `application` | `internal/service` | use cases, транзакционные сценарии, права актора |
| `infrastructure` | `internal/repository` | адаптеры портов: SQL, диалектный `codec`, маппинг ошибок |
| `infrastructure` | `internal/database` | подключение, диалекты, миграции, адаптер `UnitOfWork` |
| `presentation` | `internal/servers` | HTTP/gRPC-обработчики, DTO, маппинг, middleware, DI |

Проверка направления зависимостей — часть ревью: новый импорт инфраструктуры
(`database/sql`, драйвер, `google.golang.org/grpc`, `net/http`) в `domain` или
`model` — дефект, а не вкусовщина. Хеширование паролей живёт в
`pkg/password` (Argon2id и проверка legacy-PBKDF2), домен знает только порт
`PasswordHasher` и строку хеша.

### 1.2. Структура пакетов

```text
/cmd/            // точки входа
  api/           // HTTP/gRPC-сервер; --readycheck — проба для HEALTHCHECK
  migrate/       // применение миграций
  bootstrap/     // суперпользователь и первый тенант
/internal/
  domain/        // порты: репозитории, UnitOfWork, PasswordHasher, ошибки
  model/         // сущности, value objects, инварианты (только stdlib)
  service/       // application: use cases и транзакционные сценарии
  repository/    // infrastructure: SQL-адаптеры портов, диалектный codec
  database/      // подключение, диалекты, миграции, адаптер UnitOfWork
  servers/       // presentation: HTTP/gRPC, DTO, middleware, DI
  cache/ config/ events/ jwt/ closer/  // поддерживающие адаптеры
migrations/      // SQL-миграции: postgres/ и sqlite/
/pkg/
  password/      // Argon2id + проверка legacy-PBKDF2
  logger/        // инициализация slog, TraceHandler, otelslog
  observability/ // OpenTelemetry (traces, metrics)
  tools/         // санитизация ввода, общие утилиты
  api/ proto/    // сгенерированные контракты
```

Файлы внутри слоя группируются по бизнес-сущности из ubiquitous language
(`user.go`, `tenant.go`, `catalog.go`, `notification.go`), а не по техническим
артефактам (`controllers.go`, `models.go`, `helpers.go`). Новый пакет
появляется под бизнес-возможность, а не под технический тип.

## 2. Домен

### 2.1 Единый язык

Имена типов, методов, пакетов и ошибок MUST соответствовать терминам предметной области. Запрещены Manager, Helper, Processor, Util вне ubiquitous language.

### 2.2 Сущности

Сущность MUST иметь уникальный идентификатор и сохранять идентичность при изменении атрибутов.

```go
package order

type ID string

type Order struct {
    id      ID
    items   []item
    total   Money
    status  Status
    version int64 // для оптимистической блокировки
}

func (o *Order) ID() ID { return o.id }
```

Поля сущности SHOULD быть приватными. Изменение состояния MUST происходить через методы, сохраняющие инварианты.

### 2.3 Value Objects

Value object MUST быть неизменяемым. Валидация MUST выполняться в конструкторе.

```go
type Money struct {
    amount   int64
    currency string
}

func NewMoney(amount int64, currency string) (Money, error) {
    if amount < 0 {
        return Money{}, ErrNegativeAmount
    }
    if currency == "" {
        return Money{}, ErrEmptyCurrency
    }
    return Money{amount: amount, currency: currency}, nil
}

func (m Money) Amount() int64    { return m.amount }
func (m Money) Currency() string { return m.currency }
```

Методы "изменения" MUST возвращать новый экземпляр.

### 2.4 Агрегаты

- Агрегат MUST иметь единственный агрегатный корень.
- Внешний код MUST обращаться только к корню.
- Ссылки между агрегатами MUST быть по ID, а не по указателю.
- Инварианты MUST проверяться при каждом изменении.
- Один use case SHOULD изменять один агрегат в одной транзакции.

```go
func (o *Order) AddItem(productID ProductID, price Money) error {
    if o.status != StatusDraft {
        return ErrOrderNotEditable
    }
    o.items = append(o.items, item{productID: productID, price: price})
    o.total = o.total.Add(price)
    o.version++
    return nil
}
```

### 2.5 Доменные ошибки

Домен MUST возвращать типизированные ошибки. Ошибки инфраструктуры MUST NOT протекать в домен.

```go
var (
    ErrOrderNotFound     = errors.New("order not found")
    ErrOrderNotEditable  = errors.New("order not editable")
    ErrNegativeAmount    = errors.New("amount cannot be negative")
    ErrEmptyCurrency     = errors.New("currency cannot be empty")
)
```

Для ошибок с контекстом SHOULD использоваться `fmt.Errorf("...: %w", err)`.

### 2.6 Доменные события

Доменные события SHOULD фиксироваться внутри агрегата и публиковаться после успешного сохранения.
Для критичных сценариев MUST применяться "outbox pattern" (см. §7).

## 3. Репозитории

### 3.1 Контракт

Интерфейс MUST определяться в `domain`, реализация — в `infrastructure`.

```go
package domain

type CatalogRepository interface {
    ListPublicObjects(ctx context.Context, tenantID string, limit, offset int) ([]*model.PublicObject, error)
    GetTenantBySlug(ctx context.Context, slug string) (*model.Tenant, error)
}
```

```go
// internal/repository/catalog.go — адаптер порта.
// Алиас связывает реализацию с интерфейсом домена и документирует контракт.
type CatalogRepo = domain.CatalogRepository

type catalogRepo struct {
    db CatalogDB
    codec // плейсхолдеры и типы конкретного диалекта
}
```

- Репозиторий MUST работать с агрегатами, а не с DAO/таблицами.
- Репозиторий MUST NOT содержать бизнес-логику.
- Для отсутствующей сущности MUST возвращаться доменная ошибка (`domain.ErrNotFound`), а не `nil`, `nil`.
- Ошибки хранилища MUST транслироваться в доменные/application-ошибки (`database.MapError`), чтобы `service` и `servers` не импортировали `internal/database`.
- Потребитель (use case) MUST зависеть от интерфейса из `domain`; конкретный тип остаётся приватным для пакета `repository`.
- Если `internal/service` или `internal/servers` импортирует `internal/repository`/`internal/database` ради типа или ошибки — контракт порта нарушен.

### 3.2 PostgreSQL (pgx)

- Драйвер MUST быть `github.com/jackc/pgx/v5` (или `pgxpool`).
- Запросы MUST быть параметризованными; конкатенация строк с данными запрещена.
- Обязательно MUST передаваться `ctx`.
- Транзакциями MUST управлять `UnitOfWork` (см. §7): репозиторий берёт соединение (`conn(ctx, r.db)`) и MUST NOT открывать транзакцию сам.
- Схема БД MUST управляться миграциями (см. §3.5).
- Для оптимистической блокировки MUST использоваться поле `version` с условием `WHERE id = ... AND version = ...` и проверкой `RowsAffected`.

```go
type orderRepo struct {
    db    OrderDB
    codec // rebind: «?» → «$n» для PostgreSQL
}

func (r *orderRepo) Save(ctx context.Context, o *model.Order) error {
    const query = "UPDATE orders SET total_amount = ?, total_currency = ?, status = ?," +
        " version = version + 1 WHERE id = ? AND version = ?"

    result, err := conn(ctx, r.db).ExecContext(ctx, r.rebind(query),
        o.TotalAmount, o.TotalCurrency, o.Status, o.ID, o.Version-1)
    if err != nil {
        return fmt.Errorf("update order: %w", database.MapError(err))
    }

    return requireAffected(result, "update order")
}
```

### 3.3 SQLite (modernc.org/sqlite)

- Драйвер MUST быть `modernc.org/sqlite` (без CGO).
- Для SQLite MUST включаться `PRAGMA foreign_keys = ON` и `busy_timeout`.
- Режим WAL SHOULD включаться для конкурентного чтения.
- Транзакции MUST использовать `BEGIN IMMEDIATE` для пишущих операций, чтобы избежать `SQLITE_BUSY`.
- Плейсхолдеры — `?` или `$N` (зависит от драйвера), параметризация обязательна.

### 3.4 Двойная реализация (PostgreSQL + SQLite)

- Application-слой MUST зависеть только от интерфейсов домена, поэтому одна и та же бизнес-логика работает с обеими БД.
- Выбор драйвера MUST происходить в `internal/database/database.go` на основе конфигурации.
- SQLite SHOULD использоваться для локальной разработки, тестов и встраиваемых сценариев.
- SQL-диалекты MUST быть изолированы в своих пакетах; общий SQL MUST NOT смешиваться.
- Каждый репозиторий MUST встраивать диалектный `codec` и писать запросы с `?`: `codec.rebind` заменяет их на `$1…$n` для PostgreSQL. Сырой SQL в обход `codec` (включая тестовые хелперы) — дефект: на PostgreSQL `?` не плейсхолдер, и запрос падает с `SQLSTATE 42601`.
- Каждый запрос MUST иметь сценарий на обоих диалектах: общая функция `runXxxScenario` вызывается из `TestXxxSQLite` и `TestXxxPostgres` (PostgreSQL включается переменной `DSN_TEST`, без неё тест пропускается).

```go
type catalogRepo struct {
	db CatalogDB
	codec
}

rows, err := conn(ctx, r.db).QueryContext(ctx, r.rebind(query),
	tenantID, deletedObjectStatusID, limit, offset)
```

```go
// internal/database/database.go
switch strings.ToLower(scheme) {
  case DialectPostgres, "postgres":
    return DialectPostgres, nil
  case DialectSQLite, "sqlite":
    return DialectSQLite, nil
  case "file":
    // file: — URI-форма SQLite (file:/tmp/database.db).
    return DialectSQLite, nil
}
```

### 3.5 Миграции

- Миграции MUST храниться в `migrations/<driver>/`.
- Инструмент: `goose` (единый для проекта, `internal/database/migrate.go`).
- Файлы миграций MUST читаться с диска (`MIGRATIONS_DIR`), а не встраиваться в бинарник: каталог копируется в образ.
- Миграции MUST быть идемпотентными и обратимыми (если применимо).
- Схемы PostgreSQL и SQLite MAY различаться типами, но MUST обеспечивать одинаковые инварианты на уровне ограничений:
  * `NOT NULL` для обязательных полей;
  * `CHECK` для неотрицательных сумм и допустимых статусов;
  * уникальные индексы для естественных ключей.

## 4 Application-слой

Use case MUST:

- принимать `context.Context` первым параметром;
- загружать агрегаты через интерфейсы репозиториев;
- вызывать доменные методы;
- сохранять агрегаты;
- управлять транзакционной границей (через UnitOfWork, см. §7);
- возвращать application- или доменные ошибки в согласованном виде.

```go
type AddItemCommand struct {
    OrderID   order.ID
    ProductID order.ProductID
    Price     order.Money
}

type AddItemHandler struct {
    uow    UnitOfWork
    orders order.Repository
}

func (h *AddItemHandler) Handle(ctx context.Context, cmd AddItemCommand) error {
    return h.uow.Do(ctx, func(ctx context.Context) error {
        o, err := h.orders.FindByID(ctx, cmd.OrderID)
        if err != nil {
            return err
        }
        if err := o.AddItem(cmd.ProductID, cmd.Price); err != nil {
            return err
        }
        return h.orders.Save(ctx, o)
    })
}
```

Application-слой MUST NOT содержать бизнес-правила домена.

## 5 Presentation: HTTP (net/http)

- Роутер MUST быть `net/http`.
- Обработчики MUST NOT вызывать репозитории напрямую.
- DTO MUST NOT использоваться как доменные модели; маппинг MUST быть явным.
- Ошибки MUST транслироваться в HTTP-коды согласованно (см. таблицу ниже).
- Тело запроса MUST ограничиваться `http.MaxBytesReader`.
- `Content-Type: application/json` обязателен для JSON-эндпоинтов.

| Доменная/application-ошибка | HTTP-код |
| --- | --- |
| `ErrNotFound` (например, `ErrOrderNotFound`) | 404 |
| `ErrValidation` / `ErrNegativeAmount` / `ErrEmptyCurrency` | 400 |
| `ErrConflict` / `ErrConcurrentModification` | 409 |
| `ErrUnauthorized` | 401 |
| `ErrForbidden` | 403 |
| прочие | 500 |

```go
func (h *OrderHandler) AddItem(w http.ResponseWriter, r *http.Request) {
    var req addItemRequest
    if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid body")
        return
    }
    cmd, err := req.toCommand(r.PathValue("id"))
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if err := h.addItem.Handle(r.Context(), cmd); err != nil {
        writeAppError(w, err) // централизованный маппинг
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

- Middleware MUST включать: structured logging, recovery, timeout (по умолчанию 5–30s в зависимости от операции).
- Служебные ручки MUST быть разделены по смыслу: liveness (`GET /health`) не
  ходит во внешние зависимости, readiness (`GET /ready`) проверяет БД и
  отвечает `503`, когда её нет. Один путь для обеих проб запрещён (§14.3).
- Метаданные запроса MUST быть доступны через `ctx`.

## 6 Presentation: gRPC

- Protobuf MUST храниться в `/proto/<domain>/v1/`.
- Генерация кода MUST быть воспроизводимой (buf или protoc с зафиксированными версиями плагинов).
- Сгенерированный код MUST NOT редактироваться вручную.
- gRPC-сервер MUST возвращать канонические статусы.
- Ошибки MUST транслироваться в `status.Error` через `google.golang.org/grpc/status` и codes.
- Валидация входящих сообщений MUST выполняться до вызова use case (в т.ч. `protoc-gen-validate`, если используется).

| Доменная ошибка | gRPC code |
| --- | --- |
| `ErrNotFound` / `ErrOrderNotFound` | `codes.NotFound` |
| `ErrValidation` / `ErrNegativeAmount` | `codes.InvalidArgument` |
| `ErrConflict` / `ErrConcurrentModification` | `codes.Aborted` / `codes.FailedPrecondition` |
| `ErrUnauthorized` | `codes.Unauthenticated` |
| `ErrForbidden` | `codes.PermissionDenied` |
| прочие | `codes.Internal` |

```go
func (s *OrderServer) AddItem(ctx context.Context, req *orderv1.AddItemRequest) (*emptypb.Empty, error) {
    cmd, err := toAddItemCommand(req)
    if err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }
    if err := s.addItem.Handle(ctx, cmd); err != nil {
        return nil, mapErr(err)
    }
    return &emptypb.Empty{}, nil
}
```

- Interceptors MUST включать: recovery, logging, auth, таймауты.
- Метаданные запроса MUST быть доступны через `ctx`.

## 7 Транзакции, UoW и конкурентность

- Транзакционная граница MUST находиться в application-слое (`internal/service`), реализация — адаптер `UnitOfWork` в `infrastructure` (`internal/database`).
- Порт `UnitOfWork` MUST быть объявлен в `domain`: use case не импортирует инфраструктуру.
- Домен MUST NOT управлять транзакциями.
- Один use case SHOULD одна транзакция (если не оговорено иное).

```go
// internal/domain/unit_of_work.go
type UnitOfWork interface {
    Do(ctx context.Context, fn func(ctx context.Context) error) error
}
```

```go
// internal/service/auth.go — регистрация целиком в одной транзакции.
if err := s.uow.Do(ctx, func(ctx context.Context) error {
    if err := s.userRepo.Create(ctx, user); err != nil {
        return err
    }
    if err := s.credentialRepo.Create(ctx, credentials); err != nil {
        return err
    }
    return s.tokenRepo.Create(ctx, token)
}); err != nil {
    return nil, fmt.Errorf("register: %w", err)
}
```

- Реализация MUST использовать `github.com/avito-tech/go-transaction-manager/trm/v2` (`internal/database/uow.go`): `Do` открывает транзакцию, кладёт её в контекст и вызывает `fn`; ошибка откатывает изменения, `nil` фиксирует.
- Репозитории MUST брать соединение из контекста (`database.GetTx`, хелпер `conn(ctx, db)`) и MUST NOT открывать собственную транзакцию: иначе часть изменений зафиксируется мимо транзакции use case.
- Вложенный `Do` выполняется в уже открытой транзакции (без `SAVEPOINT`): use case MUST NOT рассчитывать на частичный откат вложенного вызова.
- Компенсирующие действия (ручное удаление уже созданных записей после ошибки) MUST NOT заменять откат.
- Для конкурентного изменения агрегатов MUST использоваться оптимистическая блокировка по `version`.
- При конфликте MUST возвращаться `ErrConcurrentModification` → 409 / `codes.Aborted`.
- Для критичных событий MUST использоваться **outbox**: событие пишется в таблицу `outbox` в той же транзакции, что и агрегат; отдельный публикатор (worker) доставляет события с at-least-once семантикой.
- Публикация в outbox MUST быть частью той же транзакции, что и сохранение агрегата.
- Для PostgreSQL SHOULD использоваться `SELECT ... FOR UPDATE` только там, где оптимистической блокировки недостаточно.

## 8. Логирование

Единственный механизм логирования — стандартный `log/slog`. Обработчик
собирается в `pkg/logger` (вывод в stdout, `TraceHandler`, мост `otelslog`).

### 8.1 Санитизация и секреты

- **Санитизируем пользовательский ввод.** Значения, пришедшие от пользователя и
  передаваемые наружу (log sinks, коллекторы, сторонние API), проходят через
  `tools.SanitizeInputString`: заменяются переводы строк и ограничивается длина.
  Это защищает от log-injection и роста объёма логов.
- **Доверенные значения не санитизируем, но помечаем.** Значения, которые
  формирует сам транспорт или фреймворк, безопасны по построению — их не
  санитизируют, но в коде отмечают комментарием с объяснением. Доверенными
  считаются:
  * HTTP-метод (`r.Method`) — валидируется `net/http` по RFC 7230 (токен без
    управляющих символов и разделителей);
  * адрес клиента (`r.RemoteAddr`, `peerAddr(ctx)`) — берётся из соединения, а не
    из тела/заголовков запроса;
  * RPC-метод (`info.FullMethod`) — значение из реестра сервисов, интерцептор
    вызывается только для точно найденного метода;
  * нормализованный маршрут/шаблон (`Request.Pattern`) — формируется роутером.

  Пример — поля access-лога в `internal/servers/accesslog.go`: санитизируются
  только `path` и `user_agent`; `method`, `remote_addr` и адрес gRPC-пира —
  доверенные. Обязательство на будущее: если в лог начнёт попадать IP из
  `X-Forwarded-For` (`HTTP_HEADER_CLIENT_IP`), значение станет
  пользовательским и его нужно санитизировать.
- **Никогда не логируем секреты.** Токены, пароли и ключи API не попадают в логи
  и наружу. Учётные данные не прогоняются через санитайзер на пути проверки —
  санитизация это операция уровня логирования, и на пути проверки креденшла она
  испортила бы валидный токен (см. `parseBearer`). Значение, идущее в лог,
  маскируется ключом (`tools.SanitizeInputString(value, "authorization")`
  вернёт `***`). Пароль в адресе подключения скрывает `redactAddr`
  (`internal/cache`). Содержимое кэша не логируем: вместо значения пишем ключ и
  длину (`internal/cache/cache.go`).

### 8.2 Контекст записей

- Логи содержат `trace_id` активной трассы: его добавляет
  `pkg/logger.TraceHandler` к записям, сделанным контекстными вызовами, если
  `trace_id` присутствует в контексте. Поэтому логируем через
  `slog.InfoContext`, `slog.DebugContext`, `slog.WarnContext`,
  `slog.ErrorContext`, передавая `context.Context`.
- `trace_id` — идентификатор активного спана из контекста записи, а не отдельно
  генерируемое значение. HTTP-запрос попадает в трассу, только если продолжает
  входящую (`traceparent`) или включена форс-трассировка; иначе запрос
  обслуживается вне трассировки, серверный спан не стартует и `trace_id` в лог не
  пишется (см. §12). Так лог не ссылается на трассу, которой нет в коллекторе.
- `span_id` в логи не пишется: для связки записи с трассой достаточно
  `trace_id`, а идентификатор спана доступен внутри самой трассы.

### 8.3 Телеметрия OpenTelemetry

- **Одна запись на сигнал.** Состояние *включения* экспорта сигнала разрешено на
  `Info` — ровно одна запись на сигнал при старте, с адресом коллектора
  (например, «otlp traces export enabled»).
- **Остальное — `Debug`.** Состояние *выключения*, детали ошибок экспортёра,
  диагностика по батчам/экспортам и дампы атрибутов логируются только на
  `Debug` и не направляются в потоки `Info`, `Warn`, `Error`.
- Адрес коллектора в записи — доверенное значение из конфигурации.

## 9. Обработка ошибок

- Ошибки оборачиваем через `%w`, проверяем через `errors.Is`/`errors.As`.
- Наружу (в HTTP/gRPC-ответ) отдаём фиксированные тексты
  (`unauthorized`, `forbidden`, `internal error`), внутренние детали — только в
  лог. Так по ответу нельзя отличить «пользователя нет» от «пароль не тот».
- Ошибку не глотаем: если её нельзя вернуть, она логируется (уровень по
  значимости) с контекстом.

## 10. Тестирование

- Покрытие бизнес-логики — не ниже 80 % (`make coverage`: `go test -coverprofile` + `go tool cover -func`, порог проверяет Makefile).
- Unit-тесты табличные, почти для каждого пакета; внешние зависимости изолируются подменяемыми переменными/интерфейсами (фейковый пул, фейковый OTLP-коллектор, подмена глобального логгера).
- Интеграционные тесты БД/кэша включаются переменными `DSN_TEST`/`REDIS_HOST`; без них они пропускаются (в CI переменные заданы).

MUST быть покрыты:

- инварианты агрегатов — unit-тестами домена (без БД);
- use cases — тестами с fake-репозиториями и fake-UoW: «Do» сразу вызывает переданную функцию, а отдельный фейк проверяет откат при ошибке;
- реализации репозиториев — интеграционными тестами для обеих БД (PostgreSQL — через testcontainers или docker-compose, SQLite — in-memory или временный файл);
- HTTP/gRPC — contract-тестами (например, httptest, bufconn).

Правила:

- Домен MUST тестироваться без БД и внешних сервисов.
- Тесты MUST проверять поведение, а не внутреннюю структуру.
- Для SQLite-тестов MUST использоваться временный файл или file::memory:?cache=shared с busy_timeout.
- Для PostgreSQL-тестов MUST использоваться контейнер с фиксированной версией образа.
- Набор интеграционных тестов MUST быть общим для обеих реализаций репозитория (table-driven по драйверу), чтобы гарантировать одинаковые инварианты.
- Внешние зависимости MUST изолироваться: HTTP-клиенты — `httptest` или интерфейс, драйвер БД — интеграционная база или узкий интерфейс (`QueryContext`/`ExecContext`), часы — функция `now` в сервисе, хеширование паролей — порт `PasswordHasher` (`pkg/password` в тестах подменяется быстрым фейком).
- Тесты MUST NOT зависеть от реального времени и сети: сроки токенов и паролей проверяются с подменёнными часами.
- Golden-файлы MAY использоваться для JSON/proto-ответов; обновление — вместе с изменением контракта, а не «чтобы тест прошёл».

## 11. Сборка и проверки

### 11.1 Цели Makefile

| Цель | Назначение |
| --- | --- |
| `build` | DEV-сборка: бинарник `./build/api` с ldflags (версия, дата, хост) |
| `build-release` | Релизная статическая сборка: `CGO_ENABLED=0`, `-trimpath`, `-s -w`, `netgo` |
| `run` | Запуск через `go run ./cmd/api` (`LOG_LEVEL=INFO`) |
| `debug` | Сборка (если нужно) и запуск `./build/api` с `LOG_LEVEL=DEBUG` |
| `test` | Тесты с покрытием и отчётом `gotest-e2e.log` |
| `coverage` | Порог покрытия (≥80 %) по `coverage.txt` |
| `lint` | `golangci-lint` + `bearer` |
| `go-check` | `govulncheck` — проверка уязвимостей зависимостей |
| `kind-test` | Полный тестовый прогон: образ dev, kind-кластер, Helm-деплой, smoke |

**Важно:** `make build` больше не релизная сборка — это DEV-бинарник с ldflags.
Релиз собирается целью `build-release`. Цель `build-debug` удалена.

### 11.2 CI

Пайплайн `.github/workflows/go.yml` запускается на изменения кода и пропускается
для документации (`paths-ignore`: `docs/**`, `**.md`). Проверки:
`golangci-lint` (версия зафиксирована), `bearer`, `govulncheck`
(`make go-check`), тесты на матрице ОС/версий Go. Дополнительно работают
`codeql.yml` и `trivy-scan.yml` (scanners: vuln/secret/misconfig).

## 12. Наблюдаемость

- **Трассировка.** W3C Trace Context + Baggage; серверный спан открывает
  `traceMiddleware` — только если запрос продолжает входящую трассу
  (`traceparent`) или включена форс-трассировка (`ENABLE_FORCE_TRACE=true`).
  Запрос без входящей трассы обслуживается вне трассировки: спан не стартует,
  поэтому в его логах нет сгенерированного `trace_id`, по которому в коллекторе
  нет трассы (см. §8.2). Имя спана — нормализованный маршрут (идентификаторы в
  пути схлопываются, см. `internal/servers/metrics.go`).
- **Метрики.** Экспорт по OTLP; инструменты создаются лениво, после
  `SetupOTelSDK`. Ключевые: длительность HTTP-запроса
  (`http.server.request.duration`), длительность gRPC-вызова
  (`rpc.server.duration`), статистика пула `pgx` (`otelpgx.RecordStats`). Метки
  маршрутов нормализованы, чтобы кардинальность не росла с числом сущностей.
- **Логи.** См. раздел 8.

## 13 Критерии приёмки

Модуль считается соответствующим спецификации, если:

- Зависимости слоёв не нарушены; domain не импортирует фреймворки и драйверы.
- Инварианты агрегатов проверяются в домене.
- Репозитории определены интерфейсами в domain.
- Use cases управляют транзакционной границей через UnitOfWork.
- Работают обе реализации репозитория (PostgreSQL и SQLite) с одинаковыми инвариантами.
- Все запросы проходят через диалектный `codec`: один и тот же SQL работает на PostgreSQL (`$1…$n`) и SQLite (`?`).
- HTTP-обработчики и gRPC-сервер не вызывают репозитории напрямую.
- Ошибки типизированы и маппятся в HTTP-коды / gRPC-статусы согласно §5–§6.
- Readiness проверяет БД (`GET /ready`), liveness не зависит от внешних зависимостей (`GET /health`).
- Есть unit-тесты домена, тесты use case и интеграционные тесты для обеих БД.
- Код проходит gofmt, go vet, golangci-lint, make lint.
- Прото-контракты версионированы и не редактируются вручную.

## 14. Kubernetes-манифесты

Манифесты (`deploy/kind/*.yaml`, шаблоны Helm-чарта `deploy/helm/bb-core`)
собираются по модели угроз **Pod Security Standards `restricted`**: под не
получает прав больше, чем нужно приложению. Соблюдение проверяет сканер
misconfig в CI (`trivy-scan.yml`, `scanners: vuln,secret,misconfig`), поэтому
отклонение от правил ниже — дефект, а не «строгость тестового стенда».

### 14.1 Обязательный `securityContext`

Pod-level (`spec.template.spec.securityContext`):

| Поле | Значение | Зачем |
| --- | --- | --- |
| `runAsNonRoot` | `true` | контейнер не может работать под root |
| `runAsUser` / `runAsGroup` | числовой UID/GID образа <= 10000 | в `scratch` нет `/etc/passwd`, а числовые значения не зависят от `gosu` в entrypoint |
| `fsGroup` | тот же GID, что у процесса | запись в `emptyDir` без root |
| `seccompProfile.type` | `RuntimeDefault` | включает штатный seccomp-профиль ядра |

Container-level (`containers[].securityContext`):

| Поле | Значение | Зачем |
| --- | --- | --- |
| `runAsNonRoot` | `true` | дублируется на контейнере: проверки сканера смотрят на контейнер |
| `allowPrivilegeEscalation` | `false` | запрет получения новых привилегий (setuid/setcap) |
| `readOnlyRootFilesystem` | `true` | корневая ФС только для чтения |
| `capabilities.drop` | `["ALL"]` | приложению capabilities не нужны |
| `seccompProfile.type` | `RuntimeDefault` | явный профиль и на уровне контейнера |

UID/GID в манифесте обязан совпадать с `USER` образа:

| Образ | UID:GID | Источник |
| --- | --- | --- |
| `bb-core` (scratch) | `55555:55555` | `Dockerfile` (`USER 55555:55555`) |
| `postgres:16-alpine` | `70:70` | пользователь `postgres` в образе |

### 14.2 Запись при `readOnlyRootFilesystem`

Read-only корень не должен ломать процесс: всё, что пишется в рантайме,
монтируется томами (`emptyDir` для тестового стенда, PVC — для постоянного).

- `bb-core` пишет только в stdout и в БД → дополнительных томов не требуется.
- PostgreSQL пишет данные, unix-сокет и временные файлы, поэтому в
  `deploy/kind/postgres.yaml` примонтированы `emptyDir` на
  `/var/lib/postgresql/data` (совпадает с `PGDATA`), `/var/run/postgresql` и
  `/tmp`.

### 14.3 Обязательные поля пода

- `automountServiceAccountToken: false` — доступ к API Kubernetes приложению не
  нужен.
- `resources.requests` и `resources.limits` для CPU и памяти.
- `readinessProbe` и `livenessProbe` с разной семантикой: readiness —
  `GET /ready` (ping БД, 503 при отказе), liveness — `GET /health` (жив ли
  процесс). Один общий путь для обеих проб запрещён: при упавшей БД под не
  выводится из балансировки, а перезапуск контейнера базу не лечит.
  `timeoutSeconds` пробы не меньше бюджета проверки (2 с на ping).
- Образ с фиксированным тегом (не `latest`), `imagePullPolicy` задан.
- Секреты — только через `secretKeyRef` (или `existingSecret` в Helm); значения
  не хранятся в манифестах и `values.yaml`.
- Запрещены `hostNetwork`, `hostPID`, `hostIPC`, `hostPath`, `privileged: true`,
  `capabilities.add` и `runAsUser: 0`.

### 14.4 Исключения

Ослабление допустимо только для тестового стенда (`deploy/kind`) и фиксируется
комментарием в манифесте с причиной и границей применимости — например, данные
в `emptyDir` теряются при пересоздании пода. Требования §14.3 не ослабляются
нигде: модель `restricted` — нижняя граница для любого манифеста в репозитории.

Ложные срабатывания эвристик сканера фиксируются в `.trivyignore.yaml`:
исключение содержит `id` (`AVD-KSV-…`), `paths` (конкретный файл) и `statement`
с обоснованием. Пример: если сканер отметит ключи `AUTH_TOKEN_TTL`/`PASSWORD_TTL`
в ConfigMap как секреты — это сроки жизни, эвристика реагирует на подстроку в
имени; настоящие секреты (DSN, `JWT_SECRET`) в ConfigMap не попадают.
CI передаёт файл сканеру (`trivyignores` в `trivy-scan.yml`); исключения без
обоснования не добавляются.

### 14.5 Проверка

```sh
# misconfig-сканеры (то же, что в CI): securityContext, capabilities, seccomp
trivy config --severity HIGH,CRITICAL deploy/

# Синтаксис и схема без кластера (kind-config.yaml — конфиг kind, не ресурс)
kubectl apply --dry-run=client -f deploy/kind/postgres.yaml
helm template bb-core deploy/helm/bb-core | kubectl apply --dry-run=client -f -

# Стенд целиком: образ, кластер, деплой, smoke-проверка /ready
make kind-test
```

Манифест с замечанием уровня HIGH не проходит пайплайн: `trivy-scan.yml`
запускается с `exit-code: 1`.
