# Микросервис бронирования отелей

## Описание проекта

Микросервис для управления бронированием ресурсов отелей (номера, виллы и т.д.). Взаимодействует с сервисом каталога синхронно (HTTP) и асинхронно (RabbitMQ).

**Функциональность:**
- Создание бронирования
- Получение бронирования по ID
- Получение статуса бронирования по ID
- Получение бронирований по фильтру с пагинацией
- Отмена бронирования

## Используемые технологии

| Компонент | Технология |
|-----------|-----------|
| Язык | Go 1.25 |
| HTTP-фреймворк | [chi](https://github.com/go-chi/chi) |
| База данных | PostgreSQL 16 |
| Очередь сообщений | RabbitMQ |
| Драйвер БД | [pgx/v5](https://github.com/jackc/pgx) |
| Миграции | [goose](https://github.com/pressly/goose) |
| Конфигурация | [envconfig](https://github.com/kelseyhightower/envconfig) |
| Контейнеризация | Docker / Docker Compose |

## Структура проекта

```
booking-service-go/
├── app/
│   ├── api/               # HTTP-хендлеры, роутер, DTO, middleware
│   │   ├── dto/
│   │   ├── handler/
│   │   └── middleware/
│   ├── clients/catalog/   # HTTP-клиент к Catalog-сервису
│   ├── config/            # Конфигурация (envconfig)
│   ├── messaging/         # RabbitMQ: connection, publisher, consumer, handlers
│   │   └── handlers/
│   ├── models/            # Доменные модели и интерфейсы
│   ├── service/           # Сервисный слой (BookingsService, BookingsQueries)
│   └── worker/            # Фоновый воркер подтверждения
├── cmd/
│   ├── booking-service/   # Точка входа приложения
│   └── migrator/          # Утилита применения миграций
├── dev/
│   └── deployments/       # Docker Compose, Dockerfile, env-файлы
├── migrations/            # SQL-миграции (goose)
├── storage/
│   └── postgres/          # Реализация репозитория (pgx)
├── swagger/               # OpenAPI 3.0 спецификация
└── Makefile
```

## Доменная модель — Статусы бронирования

```
AwaitsConfirmation ──► Confirmed ──► Cancelled
       │                                 ▲
       └─────────────────────────────────┘
```

- `awaits_confirmation` — создано, ожидает подтверждения от Catalog
- `confirmed` — подтверждено Catalog
- `cancelled` — отменено (из любого статуса; из `confirmed` только если дата начала > сегодня)

## Запуск проекта

### Предварительные требования

- Docker и Docker Compose
- Go 1.22+ (для локального запуска без Docker)

### Запуск через Make

```bash
# Запустить все сервисы (PostgreSQL, RabbitMQ, Catalog, Booking)
make up

# Применить миграции (после запуска БД)
make migrate

# Запустить юнит-тесты
make tests

# Остановить все сервисы
make down
```

### Переменные окружения

Настройки booking-service задаются в `dev/deployments/compose.dev.env`:

| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `HTTP_PORT` | `8080` | Порт HTTP-сервера |
| `POSTGRES_HOST` | `booking-db` | Хост PostgreSQL |
| `POSTGRES_PORT` | `5432` | Порт PostgreSQL |
| `POSTGRES_USER` | `booking` | Пользователь БД |
| `POSTGRES_PASSWORD` | `booking` | Пароль БД |
| `POSTGRES_DB` | `booking` | Имя БД |
| `RABBITMQ_URL` | `amqp://admin:admin@rabbitmq:5672/` | URL RabbitMQ |
| `RABBITMQ_EXCHANGE` | `booking-service` | Имя exchange |
| `CATALOG_BASE_URL` | `http://catalog-host:8080` | URL Catalog-сервиса |
| `LOG_LEVEL` | `info` | Уровень логирования |

## API

Полная OpenAPI 3.0 спецификация: [`swagger/booking-service.yaml`](swagger/booking-service.yaml)

Базовый URL (локально): `http://localhost:8081`

### Примеры запросов

#### Создание бронирования

```bash
curl -X POST http://localhost:8081/api/bookings/create \
  -H "Content-Type: application/json" \
  -d '{
    "userId": 1,
    "resourceId": 1,
    "startDate": "2026-05-01",
    "endDate": "2026-05-07"
  }'
# Ответ: {"id": 1}
```

#### Получение бронирования по ID

```bash
curl http://localhost:8081/api/bookings/1
```

#### Получение статуса бронирования

```bash
curl http://localhost:8081/api/bookings/1/status
# Ответ: {"status": "awaits_confirmation"}
```

#### Отмена бронирования

```bash
curl -X PUT http://localhost:8081/api/bookings/1/cancel
# Ответ: 204 No Content
```

#### Получение бронирований с фильтром

```bash
# Все бронирования (первая страница, 25 элементов)
curl -X POST http://localhost:8081/api/bookings/by-filter \
  -H "Content-Type: application/json" \
  -d '{"page": 1, "size": 25}'

# Фильтр по пользователю и статусу
curl -X POST http://localhost:8081/api/bookings/by-filter \
  -H "Content-Type: application/json" \
  -d '{
    "userId": 1,
    "status": "awaits_confirmation",
    "page": 1,
    "size": 10
  }'
```

#### Проверка состояния сервиса

```bash
curl http://localhost:8081/health
# Ответ: {"status": "healthy"}
```

## Асинхронное взаимодействие

После создания бронирования сервис публикует команду `booking-job.create` в RabbitMQ. Catalog-сервис обрабатывает её и публикует обратно событие `booking-job.confirmed` или `booking-job.denied`. Booking-сервис подписан на эти события и обновляет статус бронирования соответственно.

```
Booking ──[booking-job.create]──► RabbitMQ ──► Catalog
Booking ◄─[booking-job.confirmed/denied]──── RabbitMQ ◄── Catalog
```

## Статусная модель переходов

```mermaid
stateDiagram-v2
    ac: AwaitsConfirmation
    confirmed: Confirmed
    cancelled: Cancelled

    ac --> confirmed: BookingJobConfirmed (RabbitMQ)
    ac --> cancelled: Cancel / BookingJobDenied
    confirmed --> cancelled: Cancel (если startDate > today)
```
