# Changelog

Все значимые изменения проекта. Формат по [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
версии по [SemVer](https://semver.org/lang/ru/).

## Как читать версии

Проект на этапе `0.y.z`, поэтому версия читается так:

- `0.y.0` — новый цикл: крупная функциональность (фича) либо большой батч работы;
- `0.y.z` (z > 0) — только исправления найденных дефектов, без новой функциональности.

Правило репозитория: за каждым `0.y.0` следует хотя бы один релиз с настоящим
багфиксом. Так релиз нельзя объявить «идеальным» — после него обязательно что-то
находят тесты или ручная проверка.

Сообщения коммитов оставлены на английском, как в истории репозитория.

## [Unreleased]

## [0.6.1] - 2026-10-03

### Fixed

- Значение лейбла метрики экранировалось дважды: `"` и `\` проходили через
  `escapeLabelValue`, а затем ещё раз через `%q`. Prometheus читал такую строку
  как `a\"b` вместо `a"b`, то есть формат экспозиции ломался на любом значении
  с кавычкой или обратным слэшем. Теперь кавычки ставятся вручную, экранирование
  ровно одно.
- `build_info` копил серии всех версий, которыми когда-либо заменяли значение
  (`set` добавлял ключ, но не удалял прежний). Теперь гейдж хранит только текущую
  версию бинаря.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `16a6d17` | test(metrics): cover the registry counters, histograms and the text format |
| `c27c1a5` | fix(metrics): escape a label value once and keep one build_info series |

## [0.6.0] - 2026-10-03

Батч тестов: слои домена, сценариев, хранилища, конфигурации и HTTP получили
покрытие, а тесты выявили три дефекта, которые исправлены в этом же релизе.
Покрытие `internal/adapter/httpapi` — 95% операторов, `internal/config` — 100%.

### Added

- `internal/domain/booking_test.go`: инварианты брони, валидация имени и
  пользователя, правила отмены.
- `internal/domain/series_test.go`: раскладка серии по интервалу, счётчик
  вхождений, проверки границ.
- `internal/usecase/mocks_test.go`: рукописные фейки портов (часы, генератор
  идентификаторов, репозитории, логгер, счётчик операций) без mock-фреймворков.
- `internal/usecase/booking_service_test.go`: правила авторизации, конфликт,
  серия, пагинация.
- `internal/adapter/postgres/errors_test.go`: трансляция SQLSTATE в доменные
  ошибки, соглашения об именах миграций.
- `internal/config/config_test.go`: значения по умолчанию, переопределение через
  окружение, поведение при непригодном значении, единая проверка всех полей,
  `Redacted()` не выдаёт секреты.
- `internal/adapter/httpapi/*_test.go`: маршрутизация, JWT и header-аутентификация,
  middleware (request id, traceparent, access log, метрики), таблица
  «доменная ошибка → HTTP-статус», разбор query-параметров, сериализация DTO.

### Fixed

- Комната, удалённая между проверкой и вставкой брони, давала `ErrIntegrity`
  и HTTP 500. FK `bookings_room_id_fkey` переводится в `domain.ErrRoomDeleted`
  и HTTP 409 с кодом `room_deleted`: клиенту нужно повторить выбор комнаты,
  а не сообщать о внутренней ошибке сервера.
- Явно пустые `DATABASE_URL`, `HTTP_ADDR` и `AUTH_HEADER_NAME` молча
  заменялись значениями по умолчанию, поэтому проверки «не должно быть пустым»
  никогда не срабатывали. Ошибка в развёртывании (например, пустая переменная
  от шаблона) проходила незамеченной и сервис стартовал с другими настройками.
- Неизвестный путь и неверный метод отвечали текстом `404 page not found` и
  `Method Not Allowed` мимо JSON-конверта. Теперь оба случая возвращают
  `errorResponse`, а для неверного метода добавляется заголовок `Allow`.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `3aaab96` | test(domain): cover booking invariants, validation and cancel rules |
| `672348c` | test(domain): cover series occurrence layout and validation |
| `1b3e6ef` | test(usecase): cover the booking service with in-memory fakes |
| `85e217a` | fix(storage): report a room deleted mid-request as a conflict |
| `07d6f5c` | test(storage): cover sqlstate translation and migration naming |
| `7f77d84` | test(config): cover defaults, overrides and validation failures |
| `e69b45e` | fix(config): reject a blank DATABASE_URL, HTTP_ADDR or auth header name |
| `cefda00` | test(http): cover routing, credentials, middleware and the error envelope |
| `21a3585` | fix(http): answer an unknown path or a wrong method inside the json envelope |

## [0.5.0] - 2026-10-03

Наблюдаемость и эксплуатация: метрики, трассировка по `traceparent`,
структурированный access log, фоновая уборка отменённых броней и
режим проверки миграций для реплик без права менять схему.

### Added

- Реестр метрик в текстовом формате Prometheus без внешних зависимостей:
  запросы и латентность по шаблону маршрута, исходы операций бронирования,
  ошибки БД по SQLSTATE, `build_info`.
- `GET /metrics` без аутентификации: эндпоинт для сборщика, а не для клиента.
- `X-Request-Id` в ответе на каждый запрос; входящее значение сохраняется,
  если оно пригодно для логов.
- Разбор входящего `traceparent` и вынос `trace_id` в access log.
- Одна структурированная строка на запрос: метод, шаблон маршрута, статус,
  размер ответа, длительность, адрес клиента.
- Счётчики операций бронирования по исходу, счётчики ошибок БД по классу SQLSTATE.
- Порты и фоновая задача `PurgeCancelledBefore`: удаление отменённых броней
  старше окна хранения ограниченными батчами.
- `MIGRATIONS_MODE=apply|check`: проверка схемы без изменений, для реплик,
  которым запрещено мигрировать.
- Advisory lock на миграции: параллельный запуск реплик не схлопывается в гонку.
- Версия из `git describe` встраивается в бинарь через `-ldflags`.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `49429e5` | feat(platform): add a dependency free prometheus exposition registry |
| `e73ecc1` | feat(http): record request count and latency by route pattern |
| `c9535ac` | feat(http): expose the metrics endpoint in prometheus text format |
| `40ca9e3` | build(docker): stamp the version into the compiled binary |
| `0b89e5a` | feat(http): propagate a request id header on every response |
| `8ff333a` | feat(http): capture an inbound traceparent for log correlation |
| `69cf659` | feat(http): log one structured access line per request |
| `1150a90` | feat(usecase): count booking operations by outcome |
| `840826b` | feat(storage): count database errors by sqlstate class |
| `340e624` | feat(config): add the retention sweep interval and batch size |
| `3551be8` | feat(usecase): declare the cancelled booking purge port |
| `33c7b1c` | feat(storage): delete cancelled bookings in bounded batches |
| `07f3772` | feat(usecase): sweep cancelled bookings past the retention window |
| `332f0ea` | feat(storage): serialise migration runs with an advisory lock |
| `7636e32` | feat(storage): verify the schema without migrating in check mode |

## [0.5.1] - 2026-10-03

### Fixed

- Исход операции бронирования в метриках не совпадал с HTTP-статусом: ошибки
  ветки (конфликт, прошедшее время) не попадали в счётчики и учитывались как `ok`.
  Теперь считается фактический результат, а не путь кода.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `d808466` | fix(metrics): record booking outcomes on the failure paths too |

## [0.4.0] - 2026-10-03

Аутентификация и авторизация: JWT и общий секрет в заголовке, владелец брони
из `sub`, правило открытого режима.

### Added

- Контракт аутентификатора и режим `none` (аноним).
- HS256 JWT с закреплённым алгоритмом: `alg: none` и подмена алгоритма не
  проходят; проверяются `exp`, `nbf`, `iss`, `aud`.
- Аутентификация заголовками: имя пользователя + общий секрет, сравнение
  постоянного времени.
- `usecase.Principal` с флагом `Authenticated` и правилами владения:
  владелец брони — `sub`, а не строка `user_name` из тела запроса.
- Отмена брони ограничена владельцем или администратором; `ErrForbidden` → 403.
- Настройки аутентификации в конфигурации и проброс их в compose.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `2b7e129` | refactor(domain): drop sentinels that no layer produces yet |
| `d8f4020` | feat(domain): add a forbidden error for authorization failures |
| `8c8e5b2` | feat(usecase): add principal with ownership rules |
| `3d63143` | feat(usecase): derive the booking owner from the principal |
| `94d0bda` | feat(usecase): restrict cancellation to the booking owner |
| `d36bc29` | feat(http): carry the authenticated principal into the handlers |
| `c6ca083` | feat(http): add the authenticator contract and the open anonymous mode |
| `012a77c` | feat(http): add hs256 jwt authentication with pinned algorithm |
| `437d1f7` | feat(http): add shared secret header authentication |
| `07ab1a4` | feat(config): add authentication settings |
| `7d49277` | feat(http): reject api requests without a valid credential |
| `c72946b` | feat(http): map a forbidden domain error to 403 |
| `3a03729` | chore(compose): forward the authentication settings to the api |

## [0.4.1] - 2026-10-03

### Fixed

- POST-маршруты брони были зарегистрированы на корневом mux, а не на
  версионированном: `POST /api/v1/bookings` отвечал 404, а 405 на
  `POST /api/v1/bookings` не доходил до обработчика.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `a57d613` | fix(http): register the booking write routes on the versioned api mux |

## [0.3.0] - 2026-10-03

Версионирование API и постраничное чтение списка броней.

### Added

- Префикс `/api/v1` для всего API.
- Дата по умолчанию — сегодня в UTC.
- `limit` и `offset` в списке броней, потолок размера страницы.
- Коллекции отдаются голыми JSON-массивами вместо объекта-обёртки.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `8328e9e` | feat(http): serve the api under the /api/v1 prefix |
| `feddca3` | feat(http): default the date parameter to today in utc |
| `5b5dccc` | feat(storage): page the room bookings query with limit and offset |
| `2a7b446` | feat(usecase): clamp the page size to the configured maximum |
| `5059591` | feat(http): accept limit and offset query parameters |
| `169914a` | refactor(http): return collections as bare json arrays |

## [0.3.1] - 2026-10-03

### Fixed

- Отрицательный `offset` молча превращался в 0, а явный `limit=0` — в размер
  страницы по умолчанию. Оба значения теперь отвергаются с кодом
  `validation_failed` на соответствующем поле.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `9dffa54` | fix(http): reject a negative offset instead of silently clamping it |
| `b3ba177` | fix(http): treat an explicit limit of zero as the default page size |

## [0.2.0] - 2026-10-03

Повторяющиеся брони: серия вхождений с общим интервалом.

### Added

- Агрегат `Series`: счётчик, шаг, проверка границ и раскладка вхождений.
- Колонка `series_id` и частичный индекс по ней, миграция `0004`.
- Вставка всех вхождений серии одной транзакцией.
- `CreateMany` в контракте репозитория.
- Настройка `BOOKING_MAX_SERIES_OCCURRENCES`.
- Тип `Duration` в JSON и эндпоинт `POST /api/v1/bookings/series`.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `ddf0659` | feat(domain): add series aggregate with interval occurrences |
| `056bb17` | feat(storage): add series_id column and its partial index |
| `ecf9f47` | feat(storage): insert every occurrence of a series in one transaction |
| `8e26fb1` | feat(usecase): add CreateMany to the booking repository contract |
| `917647e` | refactor(config): drop the authentication block until an authenticator exists |
| `01c110f` | feat(config): add the maximum number of series occurrences |
| `64186ae` | feat(http): add a duration json type for the interval field |
| `aad2d00` | feat(http): add the recurring series endpoint |

## [0.2.1] - 2026-10-03

### Fixed

- Интервал серии короче одного вхождения приводил к наложению вхождений друг
  на друга. Такая серия отвергается с ошибкой `invalid_interval`.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `f6853a3` | fix(domain): reject a series whose interval is shorter than one occurrence |

## [0.1.1] - 2026-10-03

Первые исправления после рабочего релиза.

### Fixed

- Ошибки проверки полей возвращали 500 вместо 400.
- Идентификаторы с некорректным синтаксисом переводились в 500 вместо ошибки
  валидации домена.
- Конфигурация принимала значения вне диапазона: размер пула, таймауты, лимит
  тела запроса.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `1e8f914` | fix(http): return 400 for field errors instead of letting them become 500 |
| `c6462f7` | fix(storage): translate malformed identifiers into a domain validation error |
| `ff71822` | fix(config): fail fast on out of range pool, timeout and body settings |

## [0.1.2] - 2026-10-03

### Fixed

- Ветка ответа по ошибке поля была пустой: сервис возвращал HTTP 200 с телом,
  в котором нет ни кода ошибки, ни сообщения.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `adb9130` | fix(http): write the validation response that the field error case had dropped |

## [0.1.3] - 2026-10-03

### Fixed

- `ErrRoomNotFound` и другие точные ошибки домена попадали в общий случай
  «ошибка поля» и превращались в 400. Специальные коды проверяются раньше
  общей ветки, поэтому «нет такой комнаты» отвечает 404.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `c9e8dbc` | fix(http): keep specific error codes ahead of the field error catch-all |

## [0.1.0] - 2026-10-03

Первый рабочий сервис бронирования переговорных: домен, хранилище, HTTP и
миграции в одной транзакции.

### Added

- Домен: комната, агрегат брони с проверками и правилами отмены, ошибка конфликта
  с пересекающимися бронями.
- Платформа: часы в UTC, генератор UUID v4, логгер на `slog` (json и text).
- Конфигурация из окружения для HTTP, БД и логирования.
- Порты сценариев: репозитории брони и комнат, часы, идентификаторы, логгер.
- Сценарии: создание брони с предварительной проверкой пересечений, чтение,
  отмена, каталог комнат.
- Хранилище: пул соединений с ожиданием готовности и джиттером в откатах,
  репозитории, перевод SQLSTATE в доменные ошибки.
- Схема: ограничение `EXCLUDE USING gist` против наложений, начальный каталог
  комнат, встроенные миграции одной транзакцией.
- Транспорт: JSON-конверт ошибки с устойчивым кодом, DTO, обработчики брони и
  комнат, маршрутизация по методу, `healthz` и `readyz`.
- Точка входа: миграции, обслуживание запросов, корректное завершение.

### Commits

| Коммит | Сообщение |
| --- | --- |
| `f72666f` | chore(repo): bootstrap go module and vendor pgx |
| `c763a3f` | chore(make): add makefile with build, test and vet targets |
| `c4d45e2` | build(docker): add multi-stage image for the api |
| `87f76aa` | build(docker): add compose stack with api and postgres |
| `10be12a` | feat(domain): add room entity |
| `51ae9a6` | feat(domain): add booking aggregate with validation and cancel rules |
| `2b3ed99` | feat(domain): add conflict error carrying the blocking bookings |
| `db9a0ba` | feat(platform): add clock returning utc time |
| `7cc0004` | feat(platform): add uuid v4 id generator |
| `8a8391c` | feat(platform): add slog logger with json and text output |
| `e4c48ec` | feat(config): add environment configuration for http, database and logging |
| `b0a25c9` | feat(usecase): define repository and platform ports |
| `b0a88f1` | feat(usecase): add booking service create path with overlap preflight |
| `a5d4f47` | feat(usecase): add list, get and cancel paths plus the room catalog |
| `e733c38` | feat(storage): add postgres pool with readiness wait and jittered backoff |
| `8569ad9` | feat(storage): add booking repository and translate sqlstate violations into domain errors |
| `233eca1` | feat(storage): add room repository |
| `151403e` | feat(storage): add initial schema with the gist no-overlap constraint |
| `f80342b` | feat(storage): seed the initial room catalog |
| `5f778c7` | feat(storage): add embedded migrations applied in one transaction |
| `134ea86` | feat(http): add json error envelope with a stable error code |
| `ddf75e4` | feat(http): add dto mapping for bookings and rooms |
| `b6c06d0` | feat(http): add booking and room handlers with domain error mapping |
| `39b1f01` | feat(http): add method routing with liveness and readiness probes |
| `315cc10` | feat(app): add entrypoint that migrates, serves and shuts down gracefully |

[Unreleased]: ./CHANGELOG.md
[0.6.1]: ./CHANGELOG.md
[0.6.0]: ./CHANGELOG.md
[0.5.1]: ./CHANGELOG.md
[0.5.0]: ./CHANGELOG.md
[0.4.1]: ./CHANGELOG.md
[0.4.0]: ./CHANGELOG.md
[0.3.1]: ./CHANGELOG.md
[0.3.0]: ./CHANGELOG.md
[0.2.1]: ./CHANGELOG.md
[0.2.0]: ./CHANGELOG.md
[0.1.1]: ./CHANGELOG.md
[0.1.2]: ./CHANGELOG.md
[0.1.3]: ./CHANGELOG.md
[0.1.0]: ./CHANGELOG.md
