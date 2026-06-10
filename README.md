### Hexlet tests and linter status:

[![Actions Status](https://github.com/JackBraunYKT/go-project-316/actions/workflows/hexlet-check.yml/badge.svg)](https://github.com/JackBraunYKT/go-project-316/actions)

[![CI](https://github.com/JackBraunYKT/go-project-316/actions/workflows/ci.yml/badge.svg)](https://github.com/JackBraunYKT/go-project-316/actions/workflows/ci.yml)

## Использование

Собрать бинарный файл краулера:

```bash
make build
```

Запустить все автоматические проверки:

```bash
make test
```

Проанализировать сайт:

```bash
make run URL=https://example.com
```

Если `URL` не указан, команда выведет короткое сообщение и справку по использованию.

## Глубина обхода

Используйте CLI-флаг `-depth`, чтобы изменить количество уровней обхода внутри исходного домена:

```bash
go run ./cmd/hexlet-go-crawler -depth 2 https://example.com
```

Стартовый URL всегда попадает в отчет с `pages[].depth = 0`. Его прямые внутренние ссылки получают `pages[].depth = 1`, ссылки с этих страниц получают `pages[].depth = 2` и так далее.

Значение `-depth` задает, сколько уровней включить в отчет. Например, `-depth 1` добавляет только стартовую страницу, а `-depth 2` добавляет стартовую страницу и ее прямые внутренние ссылки. Ссылки за пределами исходного домена не добавляются в `pages`, но могут проверяться и попадать в `broken_links`, если возвращают ошибочный статус или не могут быть запрошены.

## Скорость обхода

По умолчанию краулер не добавляет искусственную задержку между HTTP-запросами.

Чтобы ограничить скорость, используйте один из CLI-флагов:

```bash
go run ./cmd/hexlet-go-crawler --delay=200ms https://example.com
go run ./cmd/hexlet-go-crawler --rps=5 https://example.com
```

`--delay` принимает формат `time.Duration`: `200ms`, `1s`, `2m`, `1h`.
`--rps` задает целевое количество запросов в секунду для всего процесса. Если указаны оба флага, `--rps` имеет приоритет над `--delay`.

При использовании пакета напрямую за скорость отвечают поля `crawler.Options.Delay` и `crawler.Options.RPS`. Положительное значение `RPS` переопределяет `Delay`; положительное значение `Delay` задает минимальный интервал между соседними HTTP-запросами всего процесса.
