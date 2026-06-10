### CI/CD Status

[![Hexlet Check](https://github.com/jobsboris27/go-project-316/actions/workflows/hexlet-check.yml/badge.svg)](https://github.com/jobsboris27/go-project-316/actions/workflows/hexlet-check.yml)
[![CI Tests](https://github.com/jobsboris27/go-project-316/actions/workflows/ci.yml/badge.svg)](https://github.com/jobsboris27/go-project-316/actions/workflows/ci.yml)

## hexlet-go-crawler

Утилита для анализа структуры веб-сайта.

## Установка

```bash
go mod download
```

## Использование

### Сборка проекта

```bash
make build
```

Бинарный файл будет создан в `bin/hexlet-go-crawler`.

### Запуск тестов

```bash
make test
```

### Запуск утилиты

```bash
make run URL=https://example.com
```

Или напрямую:

```bash
go run ./cmd/hexlet-go-crawler https://example.com
```

### Помощь

```bash
go run ./cmd/hexlet-go-crawler --help
```

### Опции

- `--depth` — глубина обхода (по умолчанию: 10)
- `--retries` — количество повторных попыток для неудачных запросов (по умолчанию: 1)
- `--delay` — задержка между запросами (например: 200ms, 1s) (по умолчанию: 0s)
- `--timeout` — таймаут запроса (по умолчанию: 15s)
- `--rps` — лимит запросов в секунду (приоритет над delay) (по умолчанию: 0 — без ограничений)
- `--user-agent` — пользовательский User-Agent
- `--workers` — количество параллельных воркеров (по умолчанию: 4)
- `--indent` — форматировать JSON вывод (по умолчанию: true)

### Глубина обхода

Параметр `--depth` определяет максимальное количество переходов по ссылкам от стартовой страницы:

- `depth = 0` — только стартовая страница
- `depth = 1` — стартовая страница + все ссылки с неё
- `depth = 2` — стартовая + ссылки с неё + ссылки со страниц второго уровня

Краулер обходит только страницы в пределах того же домена, что и стартовый URL. Внешние ссылки проверяются на доступность и попадают в `broken_links`, но не добавляются в очередь обхода.

Каждая страница появляется в отчете только один раз, даже если на неё ведут несколько ссылок.

### Ограничение скорости

Для снижения нагрузки на целевой сервер можно ограничить скорость обхода:

- `--delay=200ms` — фиксированная задержка между запросами
- `--rps=5` — лимит запросов в секунду (5 RPS = 200ms между запросами)

При указании обоих параметров приоритет имеет `--rps`. Ограничение применяется глобально ко всем запросам, а не к отдельным потокам.

### Повторные попытки

Параметр `--retries` задаёт максимальное количество повторных попыток для временных ошибок:

- Повторы выполняются только для 5xx ошибок и 429 (Too Many Requests)
- 4xx ошибки (кроме 429) не вызывают повторных попыток
- Пауза между попытками: 100ms
- При отмене контекста повторы немедленно прекращаются

## Пример вывода

```json
{
  "root_url": "https://example.com",
  "depth": 1,
  "generated_at": "2024-06-01T12:34:56Z",
  "pages": [
    {
      "url": "https://example.com",
      "depth": 0,
      "http_status": 200,
      "status": "ok",
      "error": "",
      "seo": {
        "has_title": true,
        "title": "Example Domain",
        "has_description": true,
        "description": "Example description text",
        "has_h1": true
      },
      "broken_links": [
        {
          "url": "https://example.com/missing",
          "status_code": 404,
          "error": "Not Found"
        }
      ],
      "assets": [
        {
          "url": "https://example.com/static/logo.png",
          "type": "image",
          "status_code": 200,
          "size_bytes": 12345,
          "error": ""
        },
        {
          "url": "https://example.com/static/app.js",
          "type": "script",
          "status_code": 200,
          "size_bytes": 5678,
          "error": ""
        },
        {
          "url": "https://example.com/static/style.css",
          "type": "style",
          "status_code": 200,
          "size_bytes": 2345,
          "error": ""
        }
      ],
      "discovered_at": "2024-06-01T12:34:56Z"
    }
  ]
}
```

### Описание полей JSON

**Корневой объект:**

- `root_url` — стартовый URL для обхода
- `depth` — максимальная глубина обхода
- `generated_at` — время генерации отчёта (ISO8601)
- `pages` — массив отчётов по страницам

**Страница (`pages[]`):**

- `url` — URL страницы
- `depth` — глубина страницы (0 = стартовая)
- `http_status` — HTTP-статус ответа (200, 404, 500)
- `status` — статус обработки ("ok" или "error")
- `error` — сообщение об ошибке (пустое если ok)
- `discovered_at` — время обнаружения страницы (ISO8601)
- `seo` — SEO-параметры страницы
- `broken_links` — список битых ссылок
- `assets` — список ассетов (изображения, скрипты, стили)

**SEO (`seo`):**

- `has_title` — найден ли тег `<title>`
- `title` — содержимое `<title>`
- `has_description` — найден ли `<meta name="description">`
- `description` — содержимое description
- `has_h1` — найден ли тег `<h1>`

**Битая ссылка (`broken_links[]`):**

- `url` — URL битой ссылки
- `status_code` — HTTP-статус (404, 500)
- `error` — сообщение об ошибке сети

**Ассет (`assets[]`):**

- `url` — URL ассета
- `type` — тип: "image", "script", "style"
- `status_code` — HTTP-статус
- `size_bytes` — размер в байтах
- `error` — сообщение об ошибке (пустое если ok)

## Разработка

### Запуск линтера

```bash
make lint
```

### Очистка

```bash
make clean
```
