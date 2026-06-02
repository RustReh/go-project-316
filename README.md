# hexlet-go-crawler

CLI-утилита для анализа структуры сайта.

### Hexlet tests and linter status:
[![Actions Status](https://github.com/RustReh/go-project-316/actions/workflows/hexlet-check.yml/badge.svg)](https://github.com/RustReh/go-project-316/actions)

## Требования

- Go 1.25+

## Быстрый старт

```bash
# Сборка бинарника в bin/hexlet-go-crawler
make build

# Тесты и линтер
make test

# Запуск обхода (URL обязателен)
make run URL=https://example.com
```

Запуск без URL выведет подсказку:

```bash
make run
# URL is required. Usage: make run URL=https://example.com
```

Альтернатива без Makefile:

```bash
go run ./cmd/hexlet-go-crawler --help
go run ./cmd/hexlet-go-crawler https://example.com
```

## Структура проекта

| Путь | Назначение |
|------|------------|
| `cmd/hexlet-go-crawler` | Точка входа CLI |
| `crawler` | Пакет с `Analyze(ctx, opts)` и логикой обхода |
| `bin/hexlet-go-crawler` | Собранный бинарник (`make build`) |

HTTP-запросы выполняются только через `*http.Client` из `crawler.Options.HTTPClient` — это позволяет подменять клиент в тестах.

## Глубина обхода (`--depth`)

Параметр **depth** задаёт, сколько уровней переходов по внутренним ссылкам выполняется от стартового URL:

| depth | Какие страницы попадают в отчёт |
|-------|----------------------------------|
| `1` | только стартовая страница (`depth = 0`) |
| `2` | стартовая + её прямые дочерние ссылки (`depth = 0` и `1`) |
| `3` | ещё один уровень вглубь, и т.д. |

- Стартовая страница всегда имеет **`depth: 0`** в JSON.
- Обходятся только ссылки **внутри того же хоста**, что и `root_url`.
- Внешние URL не добавляются в `pages`, но могут проверяться как обычные ссылки (в `broken_links`).
- Каждый URL встречается в `pages` **не более одного раза**, даже если в HTML есть дубликаты ссылок.

Изменить глубину:

```bash
go run ./cmd/hexlet-go-crawler --depth 2 https://example.com
make run URL=https://example.com   # по умолчанию depth=10 в CLI
```

При отмене контекста (`Ctrl+C` / `context.Cancel`) `Analyze` возвращает валидный JSON с уже собранными страницами.

## Ограничение скорости (`--delay`, `--rps`)

Лимит действует **глобально** на все HTTP-запросы краулера (страницы и проверка ссылок), а не на отдельные воркеры.

| Параметр CLI | Поле `Options` | Описание |
|--------------|----------------|----------|
| `--delay 200ms` | `Delay` | Минимальный интервал между соседними запросами |
| `--rps 5` | `RPS` | Целевое число запросов в секунду (равномерно) |

Если заданы оба параметра, **приоритет у `--rps`**. Без `delay` и `rps` искусственной задержки нет.

```bash
go run ./cmd/hexlet-go-crawler --delay 200ms https://example.com
go run ./cmd/hexlet-go-crawler --rps 5 https://example.com
```

Отмена контекста прерывает ожидание лимитера без зависания.

## Повторы запросов (`--retries`)

Параметр `--retries` задаёт максимальное число **повторных попыток** для одного HTTP-запроса (итого попыток будет `retries + 1`).

- **Повторы выполняются только для временных проблем**: сетевые ошибки, HTTP `429`, HTTP `5xx`.
- При успешном ответе (не `429`/`5xx`) повторы прекращаются.
- Между попытками всегда есть **ненулевая пауза**, чтобы не создавать бурст запросов.
- При отмене контекста дополнительные попытки прекращаются сразу.

## Ассеты (JS/CSS/изображения)

Для каждой страницы в отчёте присутствует поле `assets` — список ресурсов, найденных в HTML:

- `img[src]` → `type: "image"`
- `script[src]` → `type: "script"`
- `link[rel=stylesheet][href]` → `type: "style"`

Для каждого ассета фиксируется:

- `status_code` — HTTP-код ответа (или `0` при сетевой ошибке)
- `size_bytes` — из `Content-Length`, либо по фактическому телу при отсутствии заголовка
- `error` — пустая строка при успехе; иначе текст ошибки/статуса

Один и тот же ассет по полному URL запрашивается **только один раз** за запуск (используется кэш), даже если он встречается на разных страницах.

## Формат JSON-отчёта

Пример отчёта (все ключи присутствуют, даже если значения пустые):

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
        "title": "Example title",
        "has_description": true,
        "description": "Example description",
        "has_h1": true
      },
      "broken_links": [
        {
          "url": "https://example.com/missing",
          "status_code": 404,
          "error": "404 Not Found"
        }
      ],
      "assets": [
        {
          "url": "https://example.com/static/logo.png",
          "type": "image",
          "status_code": 200,
          "size_bytes": 12345,
          "error": ""
        }
      ],
      "discovered_at": "2024-06-01T12:34:56Z"
    }
  ]
}
```

- **`root_url`**: стартовый URL.
- **`depth`**: максимальная глубина обхода (см. раздел выше).
- **`generated_at`**: время генерации отчёта (RFC3339/ISO8601).
- **`pages`**: список страниц, каждая страница встречается не более одного раза.
- **`pages[].depth`**: расстояние (число переходов) от `root_url`.
- **`pages[].seo`**: базовые SEO-поля (`title`, `meta description`, наличие `h1`).
- **`pages[].broken_links`**: ссылки, которые недоступны (HTTP 4xx/5xx или ошибка сети).
- **`pages[].assets`**: найденные ассеты (изображения/скрипты/стили) с размерами.
- **`discovered_at`**: время, когда страница была получена/обработана.

Параметр `IndentJSON` влияет только на форматирование (пробелы/переносы строк), а содержимое остаётся тем же.

## Тестирование

Unit-тесты в `crawler/crawler_test.go` — пакет `crawler_test` (чёрный ящик): проверяются только вход `Analyze(ctx, opts)` и JSON на выходе. HTTP подменяется через `httptest.Server` или кастомный `http.Client.Transport`, без реальной сети.

Покрыты сценарии: `200 OK`, `404`, `500`, таймаут, сетевой сбой.

При push/PR автоматически запускается workflow [`.github/workflows/ci.yml`](.github/workflows/ci.yml) (`go test -race`, golangci-lint). Hexlet-проверка — в `hexlet-check.yml`.
