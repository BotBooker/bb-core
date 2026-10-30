# SLI: индикаторы уровня сервиса bb-core

| Поле | Значение |
| --- | --- |
| Версия | 0.1 |
| Статус | Draft |
| Область | `bb-core`: HTTP `:8080`, gRPC `:50051`, PostgreSQL |
| Связанные документы | [SLO.md](SLO.md), [SLA.md](SLA.md), [план внедрения](../plans/reliability-gaps-2026-10-30.md) |
| Последнее обновление | 2026-10-30 |

Документ отвечает на вопрос **«что и как измеряется»**. Целевые значения
находятся в [SLO.md](SLO.md), обязательства перед клиентом — в [SLA.md](SLA.md).

RFC-ключевые: MUST — обязательно, SHOULD — рекомендуется, MAY — допустимо.

## 1. Назначение и границы

SLI (service level indicator) — количественная характеристика уровня сервиса,
измеряемая по сырым данным. SLI не содержит целей: одна и та же величина может
использоваться в SLO с разными окнами и порогами.

**В области документа:**

- серверное поведение `bb-core` — HTTP-ручки и унарные gRPC-методы;
- доступность PostgreSQL в той мере, в какой её видит приложение (`/ready`,
  пул `pgx`);
- процессные сигналы, от которых зависит обещание клиенту (бэкапы, проверка
  восстановления).

**Вне области:**

- сеть клиента, Telegram/WhatsApp, веб-панель и платёжные провайдеры — это
  другие репозитории и другие SLA;
- инфраструктурные метрики кластера, кроме сатурации, перечисленной в SLI-7;
- бизнес-показатели бронирований и платежей — соответствующих метрик пока нет
  (см. [план внедрения](../plans/reliability-gaps-2026-10-30.md), G9).

## 2. Термины и соглашения

| Термин | Значение |
| --- | --- |
| Бизнес-запрос | HTTP-запрос к `/api/v1/*`, `/auth/*`, `/users/me`, `/t/*` или унарный gRPC-вызов бизнес-сервиса |
| Хороший запрос | Запрос, завершившийся без серверной ошибки: HTTP-статус не `5xx`; для gRPC — код не из `2, 8, 13, 14, 15` |
| Знаменатель | Все бизнес-запросы окна после исключений из §5 |
| Окно оценки | Интервал, по которому считается отношение (в SLO — 28 дней, шаг 5 минут) |
| Группа маршрутов | Набор ручек с одинаковой семантикой: read, auth, write, public, service |
| Route label | Нормализованный маршрут из `http.route` / `rpc.method`; идентификаторы схлопнуты |
| Минимальный объём | 1000 бизнес-запросов за окно; ниже — SLI считается по внешней пробе (SLI-9) |
| Пропуск данных | Отсутствие серий метрики дольше 5 минут при работающем приложении |

Соглашения:

- SLI MUST быть отношением двух счётчиков или гистограммой, а не «средним по
  логам»: логи используются для разбора инцидентов и сверки.
- SLI MUST использовать нормализованные маршруты, чтобы кардинальность не росла
  с числом сущностей.
- SLI MUST исключать служебные ручки (`/health`, `/ready`,
  `/api/swagger/*`) из доступности бизнес-ручек; готовность измеряется отдельно
  (SLI-5).
- SLI MUST NOT содержать `tenant_id`, `user_id`, IP-адрес или иные
  высококардинальные и персональные метки.

## 3. Источники данных

| Источник | Что даёт | Где | Задержка | Роль |
| --- | --- | --- | --- | --- |
| OTel-метрики приложения | `http.server.request.duration`, `rpc.server.duration`, статистика пула `pgx` | OTLP/gRPC → Collector → Prometheus | до 60 с + scrape | основной источник SLI |
| Access-логи | метод, маршрут, статус, длительность | stdout и OTLP-логи | секунды | сверка и разбор инцидентов |
| `/ready` | ping БД с таймаутом 2 с | Kubernetes-проба и Docker HEALTHCHECK | 10–30 с | SLI-5 |
| `otelpgx` | счётчики и состояния пула | OTel-метрики | до 60 с | SLI-7 |
| kube-state-metrics / cAdvisor | CPU, память, готовые реплики | кластерный мониторинг | 15–30 с | guardrails SLI-7 |
| Артефакты бэкапов | время запуска, результат, длительность restore | job-логи и хранилище | минуты | SLI-8 |
| Внешняя синтетическая проба | доступность и latency снаружи | отдельный prober | 30 с | SLI-9, SLA |

**Важно:** у приложения нет собственного `/metrics`; Prometheus получает данные
на стороне коллектора. Имена метрик после конвертации OTel → Prometheus зависят
от конфигурации коллектора. В документе указаны **ожидаемые** имена; перед
вводом SLO они MUST быть подтверждены на стенде (задача G11 в
[план внедрения](../plans/reliability-gaps-2026-10-30.md)).

## 4. Каталог SLI

Общие правила:

- метка `service_name="bb-core"` — из resource attribute `service.name`;
- окно вычисления — 5 минут, если не указано иное;
- 4xx — ошибка клиента и считается «хорошим» запросом для доступности, 5xx —
  «плохим»;
- запросы с `http.route="unmatched"` исключаются: это сканирование и битые URL;
- SLI MUST NOT быть единственным источником при полном отказе: если запросы не
  доходят до приложения, знаменатель пуст (см. §5).

Владелец SLI — backend-команда; эксплуатационные SLI-5…SLI-9 (готовность,
сатурация, бэкапы, пробы) ведутся SRE/эксплуатацией.

### 4.1. SLI-1. Доступность HTTP (бизнес-ручки)

| Поле | Значение |
| --- | --- |
| Вопрос | Какая доля бизнес-запросов завершилась без серверной ошибки? |
| Формула | `good / total` |
| Числитель | `rate(http_server_request_duration_seconds_count{status!~"5.."})` |
| Знаменатель | `rate(http_server_request_duration_seconds_count)` |
| Метки | `http_request_method`, `http_route`, `http_response_status_code`, `service_name` |
| Исключения | `/health`, `/ready`, `/api/swagger/*`, `unmatched` |
| Статус | ✅ измеримо текущими метриками |

```promql
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core",
  http_route!~"/health|/ready|/api/swagger/.*|unmatched",
  http_response_status_code!~"5.."}[5m]))
/
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core",
  http_route!~"/health|/ready|/api/swagger/.*|unmatched"}[5m]))
```

Особенности:

- `metricsMiddleware` стоит внутри `authenticate`: запросы, отклонённые
  аутентификацией (401/403), в метрику не попадают и видны только в
  access-логе. Для доступности это допустимо (ошибки клиента), но для анализа
  атак — пробел G16;
- 5xx сегодня — это `500` из `writeServiceError` и `503` от `/ready`; последний
  исключён из SLI-1 и учитывается в SLI-5;
- паника в хендлере сейчас **не попадает** в метрику и access-лог: в
  `metricsMiddleware` нет `defer`, запрос исчезает из знаменателя и доступность
  завышается. Это блокер G2, а не свойство SLI.

### 4.2. SLI-2. Доступность gRPC

| Поле | Значение |
| --- | --- |
| Вопрос | Какая доля унарных gRPC-вызовов завершилась без серверной ошибки? |
| Формула | `good / total` |
| Числитель | Запрос, код которого не входит в серверный набор (см. ниже) |
| Знаменатель | `rate(rpc_server_duration_seconds_count)` |
| Метки | `rpc_method`, `rpc_grpc_status_code`, `rpc_system` |
| Исключения | `/grpc.health.v1.Health/*`, `/grpc.reflection.*` |
| Статус | ✅ измеримо текущими метриками |

Серверные коды: `Unknown (2)`, `ResourceExhausted (8)`, `Internal (13)`,
`Unavailable (14)`, `DataLoss (15)`. Клиентские коды (`InvalidArgument`,
`NotFound`, `Unauthenticated`, `PermissionDenied`, `FailedPrecondition`,
`OutOfRange`, `Canceled`, `DeadlineExceeded`) в бюджет ошибок не входят.
`DeadlineExceeded` разбирается по трассам: если дедлайн ставит сервер, код
считается серверной ошибкой.

Инструментированы только унарные вызовы: стриминговых сервисов сейчас нет
(`NewMetricsInterceptor` — унарный интерцептор). При появлении стриминга SLI
MUST быть расширен отдельно.

### 4.3. SLI-3. Latency HTTP

| Поле | Значение |
| --- | --- |
| Вопрос | Какая доля запросов уложилась в порог? |
| Формула | `count(le=threshold) / count` для группы маршрутов |
| Источник | `http_server_request_duration_seconds_bucket` |
| Метки | `http_request_method`, `http_route`, `http_response_status_code`, `le` |
| Исключения | 5xx, `/health`, `/ready`, `/api/swagger/*`, `unmatched` |
| Статус | 🚧 требует явных границ гистограммы (G1) |

Группы и пороги (порог фиксируется в SLO, здесь — только измерение):

| Группа | Маршруты | Метод | Порог |
| --- | --- | --- | --- |
| read | `/api/v1/*`, `/users/me`, `/auth/tenants`, `/t/*` | `GET` | 300 мс |
| auth | `/auth/register`, `/auth/login`, `/auth/password` | `POST` | 750 мс |
| write | остальные `/api/v1/*` | `POST`, `PATCH`, `DELETE` | 500 мс (SLO пока нет) |

```promql
sum(rate(http_server_request_duration_seconds_bucket{
  service_name="bb-core",
  http_route=~"/api/v1/.*|/users/me|/auth/tenants|/t/",
  http_request_method="GET",
  http_response_status_code!~"5..",
  le="0.3"}[5m]))
/
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core",
  http_route=~"/api/v1/.*|/users/me|/auth/tenants|/t/",
  http_request_method="GET",
  http_response_status_code!~"5.."}[5m]))
```

**Блокер:** приложение пишет длительность в секундах, а SDK по умолчанию
применяет границы `[0, 5, 10, 25, …, 10000]` к записанным значениям, не
пересчитывая их по единице измерения. Пока границы не заданы явно, все ответы
быстрее 5 секунд лежат в одном бакете, и `le="0.3"` не существует. До мержа G1
SLI-3 определён, но неизмерим.

### 4.4. SLI-4. Latency gRPC

| Поле | Значение |
| --- | --- |
| Вопрос | Какая доля унарных вызовов уложилась в порог? |
| Формула | `count(le=threshold) / count` |
| Источник | `rpc_server_duration_seconds_bucket` |
| Метки | `rpc_method`, `rpc_grpc_status_code`, `le` |
| Исключения | health, reflection, клиентские коды |
| Статус | 🚧 требует явных границ гистограммы (G1) |

Порог согласован с SLO-4 (300 мс); отдельная цель для gRPC не вводится, пока
нет достаточного трафика. `DeadlineExceeded` и `Canceled` исключаются из
числителя и знаменателя: клиент сам ограничил время вызова.

### 4.5. SLI-5. Готовность и доступность БД

| Поле | Значение |
| --- | --- |
| Вопрос | Отвечает ли приложение «готов обслуживать» и доступна ли БД? |
| Формула | `ready_200 / ready_total` |
| Источник | `http_server_request_duration_seconds_count{http_route="/ready"}` |
| Метки | `http_response_status_code` |
| Исключения | нет |
| Статус | ✅ измеримо, но с оговоркой о полном отказе |

```promql
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core", http_route="/ready",
  http_response_status_code="200"}[5m]))
/
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core", http_route="/ready"}[5m]))
```

Оговорки:

- пробу опрашивает kubelet (10 с) или Docker HEALTHCHECK (30 с); при одной
  реплике это 2–6 точек в минуту — отношение грубое, но пригодное для окна
  28 дней;
- если процесс умер, `/ready` не отвечает и серии метрики нет: отношение не
  определено, а не равно нулю. Полный отказ MUST ловиться внешней пробой
  (SLI-9) и состоянием реплик в кластере;
- `/ready` проверяет только `PingContext` (2 с), а не выполнение бизнес-запроса:
  деградация БД без потери ping видна в latency SLO, а не здесь.

### 4.6. SLI-6. Доступность публичного каталога

| Поле | Значение |
| --- | --- |
| Вопрос | Доступен ли анонимный путь, по которому приходят пользователи бота? |
| Формула | `good / total` для публичных маршрутов |
| Маршруты | `/api/v1/public/*`, `/t/*` |
| Метки | `http_route`, `http_response_status_code` |
| Исключения | 4xx, 5xx считается плохим |
| Статус | ✅ измеримо текущими метриками |

Это подмножество SLI-1, выделенное в отдельный SLI: публичный каталог —
критический путь входа клиента, и его деградация не должна растворяться в
общем трафике. Формула повторяет SLI-1 с фильтром:

```promql
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core",
  http_route=~"/api/v1/public/.*|/t/",
  http_response_status_code!~"5.."}[5m]))
/
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core",
  http_route=~"/api/v1/public/.*|/t/"}[5m]))
```

### 4.7. SLI-7. Сатурация

| Поле | Значение |
| --- | --- |
| Вопрос | Не приблизились ли мы к пределу пула, CPU или памяти? |
| Источник | `pgxpool.*`, kube-state-metrics/cAdvisor |
| Статус | 🚧 пул — ✅, CPU/память зависят от кластерного мониторинга |

Показатели:

| Показатель | Формула | Смысл |
| --- | --- | --- |
| Утилизация пула | `pgxpool_acquired_connections / pgxpool_max_connections` | близость к исчерпанию соединений |
| Пустые acquire | `rate(pgxpool_empty_acquires_total[5m])` | запросы ждали свободное соединение |
| Среднее ожидание | `rate(pgxpool_empty_acquire_wait_time_seconds_total[5m]) / rate(pgxpool_empty_acquires_total[5m])` | цена ожидания |
| Отменённые acquire | `rate(pgxpool_canceled_acquires_total[5m])` | отмены из-за дедлайнов |
| CPU | `container_cpu_usage_seconds_total` по контейнеру | близость к limit |
| Память | `container_memory_working_set_bytes` | риск OOM |

Прямой метрики «сколько хешей считается сейчас» пока нет
(G10); до её появления saturation по памяти оценивается косвенно.

### 4.8. SLI-8. Долговечность и восстановление

| Поле | Значение |
| --- | --- |
| Вопрос | Можно ли восстановить данные в обещанные RPO/RTO? |
| Источник | job-логи бэкапов, журнал restore-учений |
| Статус | ⬜ требуется автоматизация (G3) |

Показатели:

- доля успешных бэкапов за календарный месяц: `successful / scheduled`,
  цель — 100 %;
- фактический RPO — максимальный интервал между успешными копиями, цель — 4 ч;
- фактический RTO — длительность восстановления на отдельной базе, цель — 4 ч;
- restore-учение — не реже одного раза в квартал, результат фиксируется в
  [RUNBOOK.md](../RUNBOOK.md).

### 4.9. SLI-9. Ключевые сценарии (внешняя проба)

| Поле | Значение |
| --- | --- |
| Вопрос | Работает ли сервис снаружи, а не только «изнутри»? |
| Источник | синтетический prober, ≥ 2 локации |
| Статус | ⬜ пробы отсутствуют (G8) |

Минимальный набор:

- `GET /api/v1/public/tenants/{canary}` — 200 и ожидаемый `slug`;
- `GET /ready` — 200;
- `POST /auth/login` с канареечной учётной записью — 200 и непустой токен
  (учётная запись служебная; сразу после проверки — `logout`, чтобы не
  накапливать сессии);
- `grpc.health.v1.Health/Check` и `auth.v1.AuthService/GetCurrentUser` с
  канареечным токеном — `SERVING` и профиль.

Успех — код 2xx и ожидаемое тело, таймаут 2 с. Публичный каталог и `/ready`
проверяются каждые 30 с, логин — раз в 5 минут с немедленным `logout`, чтобы
не накапливать сессии. Проба MUST NOT входить в SLI-1, чтобы не считать один
запрос дважды. Именно SLI-9 ловит полный отказ, при котором серверные метрики
молчат.

## 5. Исключения и политика данных

| Что | Доступность (SLI-1/2) | Latency (SLI-3/4) | Готовность (SLI-5) |
| --- | --- | --- | --- |
| `/health`, `/ready`, Swagger | исключается | исключается | `/ready` — источник |
| `unmatched` (404 сканера) | исключается | исключается | — |
| 4xx | входит как «хороший» | входит, если не 5xx | — |
| 5xx | «плохой» | исключается | — |
| 429 (после rate limiting) | исключается до отдельного решения | исключается | — |
| Плановые работы | входят в SLO | входят в SLO | входят в SLO |

- **SLO считается по всем периодам, включая плановые работы.** Инженерная цель
  не должна улучшаться за счёт «мы же предупреждали».
- **SLA исключает анонсированные плановые работы** (см. [SLA.md](SLA.md)).
- **Пропуск данных.** Для SLA пропуск дольше 5 минут считается недоступностью.
  Для SLO пропуск фиксируется как инцидент наблюдаемости и не может молча
  улучшать показатель: окно помечается `no_data`, разбор обязателен.
- **Полный отказ.** Request-based SLI видит только дошедшие запросы. Если
  ingress, DNS или процесс не отвечают, знаменатель пуст и отношение не
  определено. Поэтому SLI-1/2 MUST дополняться SLI-9; в SLA доступность
  измеряется внешней пробой, а серверные SLI — диагностика.
- **Минимальный объём.** Если за 28 дней меньше 1000 бизнес-запросов, SLI-1/2
  не считаются: используется SLI-9/готовность. Иначе «100 % на трёх запросах»
  создаёт ложную уверенность.
- **Новые и beta-ручки** попадают в SLO через 30 дней после включения в
  production: сначала нужна статистика и подтверждение, что они не искажают
  картину.

## 6. Кардинальность и приватность

- Метки SLI MUST NOT содержать `tenant_id`, `user_id`, e-mail, IP-адрес,
  `trace_id` и другие высококардинальные или персональные значения.
- Идентификаторы в путях схлопываются: `/api/v1/users/{uuid}`, а не конкретный
  UUID. Нормализация выполняется в `internal/servers/metrics.go`
  (`normalizePath`, `routeLabel`).
- Для gateway-ручек метка маршрута — нормализованный путь; для верхнего mux —
  шаблон `Request.Pattern`; для всего, что не совпало, — `unmatched`.
- Пользовательские значения в метки не попадают вовсе: только доверенные
  атрибуты транспорта и нормализованные маршруты.
- Кардинальность проверяется при добавлении новой метки: одна метка не должна
  давать больше ~1000 серий на инстанс.

## 7. Проверка качества SLI

SLI готов к использованию в SLO, если выполнены все пункты:

1. Определение и формула записаны в этом документе.
2. Источник данных стабилен: метрика экспортируется, имя подтверждено на
   стенде.
3. Формула проверена на реальных данных и даёт осмысленные значения на
   health-трафике и под нагрузкой.
4. Исключения перечислены и не оставляют «слепых зон» (полный отказ, паника,
   пропуск данных).
5. Есть сверка с access-логами: расхождение количества запросов больше 1 % за
   час разбирается.
6. Назначен владелец, есть дашборд и алерт (или явная запись в
   [план внедрения](../plans/reliability-gaps-2026-10-30.md)).

Ежемесячная сверка: сумма `http_server_request_duration_seconds_count` по всем
маршрутам сравнивается с числом записей access-лога за тот же период. Это ловит
потерянные серии, двойной учёт и ошибки фильтров.

## Приложение A. PromQL

Все запросы предполагают, что имена метрик после OTel → Prometheus подтверждены
(приложение C). Если коллектор называет метрики иначе, меняется только имя.

```promql
# SLI-1: доступность HTTP
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core",
  http_route!~"/health|/ready|/api/swagger/.*|unmatched",
  http_response_status_code!~"5.."}[5m]))
/
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core",
  http_route!~"/health|/ready|/api/swagger/.*|unmatched"}[5m]))

# SLI-2: доступность gRPC
sum(rate(rpc_server_duration_seconds_count{
  service_name="bb-core",
  rpc_method!~"/grpc.health.*|/grpc.reflection.*",
  rpc_grpc_status_code!~"2|8|13|14|15"}[5m]))
/
sum(rate(rpc_server_duration_seconds_count{
  service_name="bb-core",
  rpc_method!~"/grpc.health.*|/grpc.reflection.*"}[5m]))

# SLI-3: latency чтения (порог 300 мс)
sum(rate(http_server_request_duration_seconds_bucket{
  service_name="bb-core",
  http_route=~"/api/v1/.*|/users/me|/auth/tenants|/t/",
  http_request_method="GET",
  http_response_status_code!~"5..",
  le="0.3"}[5m]))
/
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core",
  http_route=~"/api/v1/.*|/users/me|/auth/tenants|/t/",
  http_request_method="GET",
  http_response_status_code!~"5.."}[5m]))

# SLI-5: готовность
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core", http_route="/ready",
  http_response_status_code="200"}[5m]))
/
sum(rate(http_server_request_duration_seconds_count{
  service_name="bb-core", http_route="/ready"}[5m]))

# SLI-7: утилизация пула
pgxpool_acquired_connections / pgxpool_max_connections

# SLI-7: среднее ожидание соединения
rate(pgxpool_empty_acquire_wait_time_seconds_total[5m])
/
rate(pgxpool_empty_acquires_total[5m])
```

## Приложение B. Маршруты и группы

| Маршрут | Метка `http.route` | Группа | В доступности |
| --- | --- | --- | --- |
| `GET /health` | `/health` | service | нет |
| `GET /ready` | `/ready` | service | нет (SLI-5) |
| `GET /users/me` | `/users/me` | read | да |
| `POST /auth/register` | `/auth/register` | auth | да |
| `POST /auth/login` | `/auth/login` | auth | да |
| `POST /auth/password` | `/auth/password` | auth | да |
| `POST /auth/logout` | `/auth/logout` | auth | да |
| `GET /auth/tenants` | `/auth/tenants` | read | да |
| `POST /auth/tenant/switch` | `/auth/tenant/switch` | auth | да |
| `POST /auth/logout-all` | `/auth/logout-all` | auth | да |
| `GET /t/{slug}` | `/t/` | public | да |
| `/api/v1/*` | нормализованный путь, например `/api/v1/users/{uuid}` | read/auth/write/public | да |
| `/api/swagger/*` | `/api/swagger/` | service | нет |
| не совпало | `unmatched` | — | нет |

## Приложение C. OTel → Prometheus

| OTel-инструмент | Тип и единица | Ожидаемое имя в Prometheus | Метки |
| --- | --- | --- | --- |
| `http.server.request.duration` | histogram, `s` | `http_server_request_duration_seconds_{bucket,sum,count}` | `http_request_method`, `http_route`, `http_response_status_code` |
| `rpc.server.duration` | histogram, `s` | `rpc_server_duration_seconds_{bucket,sum,count}` | `rpc_method`, `rpc_grpc_status_code`, `rpc_system` |
| `pgxpool.acquired_connections` | up-down counter | `pgxpool_acquired_connections` | `db_client_connection_pool_name` |
| `pgxpool.max_connections` | gauge | `pgxpool_max_connections` | то же |
| `pgxpool.empty_acquires` | counter | `pgxpool_empty_acquires_total` | то же |
| `pgxpool.empty_acquire_wait_time` | counter, `ns` | `pgxpool_empty_acquire_wait_time_seconds_total` | то же |
| `pgxpool.canceled_acquires` | counter | `pgxpool_canceled_acquires_total` | то же |

Проверка имён на стенде:

```promql
group by (__name__) ({__name__=~"http_server_request_duration.*"})
group by (__name__) ({__name__=~"rpc_server_duration.*"})
group by (__name__) ({__name__=~"pgxpool_.*"})
```

Суффикс `_total` у счётчиков и `_seconds` у единиц добавляет экспортёр/приёмник
Prometheus; конкретика зависит от конфигурации коллектора. Единицу `ns` часть
экспортёров сохраняет как `_nanoseconds`, поэтому имя
`pgxpool_empty_acquire_wait_time_seconds_total` тоже проверяется на стенде
(G11). До подтверждения имена в этом документе — ожидаемые, а не
гарантированные.

## Приложение D. Скрипт нагрузочного тестирования (k6)

```javascript
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    constant_request_rate: {
      executor: 'constant-arrival-rate',
      rate: 100,              // Целевой RPS
      timeUnit: '1s',         // В секунду
      duration: '2m',         // Длительность теста (например, 2 минуты)
      preAllocatedVUs: 50,    // Сколько виртуальных пользователей выделить сразу
      maxVUs: 200,            // Максимальный лимит пользователей при задержках
    },
  },
  thresholds: {
    // Проверка критерия успешности: p95 latency должен быть строго меньше 300 мс
    http_req_duration: ['p(95)<300'],
    // Дополнительный критерий: процент ошибок должен быть близким к нулю
    http_req_failed: ['rate<0.01'],
  },
};

export default function () {
  // Замените URL на ваш целевой эндпоинт
  const res = http.get('http://your-server-ip:port/api/target-endpoint');

  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}
```
