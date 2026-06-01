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

## Тестирование

Unit-тесты в `crawler/crawler_test.go` — пакет `crawler_test` (чёрный ящик): проверяются только вход `Analyze(ctx, opts)` и JSON на выходе. HTTP подменяется через `httptest.Server` или кастомный `http.Client.Transport`, без реальной сети.

Покрыты сценарии: `200 OK`, `404`, `500`, таймаут, сетевой сбой.

При push/PR автоматически запускается workflow [`.github/workflows/ci.yml`](.github/workflows/ci.yml) (`go test -race`, golangci-lint). Hexlet-проверка — в `hexlet-check.yml`.
