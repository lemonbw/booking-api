# booking-api

Сервис бронирования переговорных комнат на Go. Чистая архитектура, PostgreSQL 17,
Prometheus-метрики без внешних зависимостей, JWT и header-аутентификация.

## Быстрый старт

```bash
docker compose up -d --build
curl localhost:8080/readyz
curl localhost:8080/api/v1/rooms
```

Создать бронь:

```bash
curl -X POST localhost:8080/api/v1/bookings \
  -H 'content-type: application/json' \
  -d "{\"room_id\":1,\"user_name\":\"ada\",\"start_time\":\"$(date -u -d '+1 day' +%Y-%m-%dT09:00:00Z)\",\"end_time\":\"$(date -u -d '+1 day' +%Y-%m-%dT10:00:00Z)\"}"
```

Локально без Docker:

```bash
docker compose up -d db
make build
DATABASE_URL="postgres://booking:booking@localhost:5432/booking?sslmode=disable" ./bin/booking-api
```

## API

Все маршруты под `/api/v1`. Коллекции отдаются голыми JSON-массивами.

| Метод | Путь | Назначение |
| --- | --- | --- |
| `POST` | `/api/v1/bookings` | одна бронь |
| `POST` | `/api/v1/bookings/series` | серия с интервалом, атомарно |
| `GET` | `/api/v1/bookings?room_id=1&date=2026-03-03&limit=50&offset=0` | брони комнаты за день |
| `GET` | `/api/v1/bookings/{id}` | одна бронь |
| `POST` | `/api/v1/bookings/{id}/cancel` | отмена |
| `GET` | `/api/v1/rooms` | каталог комнат |
| `GET` | `/api/v1/rooms/{id}` | одна комната |
| `GET` | `/healthz` | liveness |
| `GET` | `/readyz` | readiness, проверяет БД |
| `GET` | `/metrics` | метрики, без аутентификации |

Ошибка всегда в одном конверте:

```json
{"error": "booking_conflict", "field": "time_range", "message": "..."}
```

| Код | Когда |
| --- | --- |
| `400` | ошибка валидации, неизвестное поле в теле |
| `401` | нет или неверная учётная запись |
| `403` | чужую бронь |
| `404` | нет такой брони или комнаты |
| `405` | метод не поддерживается маршрутом |
| `409` | пересечение, уже началась, комната удалена |
| `413` | тело больше `HTTP_MAX_BODY_BYTES` |
| `500` | внутренняя ошибка, детали не раскрываются |
| `503` | база недоступна |

## Настройки

Всё читается из окружения, значения по умолчанию — рабочие.

| Переменная | По умолчанию | Смысл |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | адрес прослушивания |
| `HTTP_MAX_BODY_BYTES` | `65536` | предел тела запроса |
| `DATABASE_URL` | локальный postgres | строка подключения |
| `DB_MAX_CONNS` / `DB_MIN_CONNS` | `10` / `1` | размер пула |
| `DB_WAIT_TIMEOUT` | `60s` | сколько ждать готовности БД при старте |
| `AUTH_MODE` | `none` | `none`, `jwt` или `header` |
| `AUTH_JWT_SECRET` | — | не меньше 32 символов при `AUTH_MODE=jwt` |
| `AUTH_HEADER_NAME` | `X-User-Name` | имя заголовка с пользователем |
| `AUTH_HEADER_SECRET` | — | общий секрет, не меньше 32 символов |
| `BOOKING_MAX_SERIES_OCCURRENCES` | `52` | потолок вхождений серии |
| `BOOKING_PAGE_SIZE` | `100` | размер страницы по умолчанию |
| `MIGRATIONS_MODE` | `apply` | `apply` или `check` (только проверить схему) |
| `RETENTION_CANCELLED` | `2160h` | сколько хранить отменённые |
| `PURGE_INTERVAL` / `PURGE_BATCH_SIZE` | `1h` / `500` | параметры фоновой уборки |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | логирование |

Пустая `DATABASE_URL`, `HTTP_ADDR` или `AUTH_HEADER_NAME` — ошибка запуска, а не
тихая подстановка умолчания.

## Разработка

```bash
make build          # бинарь с версией из git describe
make test           # go test -race
make vet            # go vet
make fmt            # gofmt -s
make up / make down # стек в Docker
```

Интеграционные тесты хранилища пропускают себя без `DATABASE_URL` и создают
свои записи, которые удаляют сами:

```bash
docker compose up -d db
DATABASE_URL="postgres://booking:booking@localhost:5432/booking?sslmode=disable" make test
```

## Структура

```
cmd/app                точка входа: только сборка зависимостей
internal/domain        сущности и доменные ошибки, только стандартная библиотека
internal/usecase       сценарии и правила, зависят от портов
internal/adapter/httpapi    транспорт: маршруты, DTO, middleware, аутентификация
internal/adapter/postgres   SQL, миграции, перевод SQLSTATE в доменные ошибки
internal/platform      часы, генератор id, логгер, метрики, версия
```

Правила, которые держит проект: домен не знает о других пакетах, HTTP-слой не
принимает бизнес-решений, SQL собирается только из параметров, комментариев в
коде нет.

## Наблюдаемость

- `/metrics`: `booking_api_http_requests_total`, `booking_api_http_request_duration_seconds`,
  `booking_api_booking_operations_total`, `booking_api_database_errors_total`,
  `booking_api_rate_limited_total`, `booking_api_build_info`.
- Каждый ответ содержит `X-Request-Id`; входящий `traceparent` попадает в лог.
- Одна структурированная строка на запрос: метод, шаблон маршрута, статус,
  байты, длительность, клиент.

## Лицензия и история

История изменений по версиям — в [CHANGELOG.md](CHANGELOG.md).
