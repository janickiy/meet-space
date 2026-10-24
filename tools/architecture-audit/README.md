# Инвентаризация архитектуры

`go run ./tools/architecture-audit` emits the production `internal/` package
inventory as JSON: imports, inbound dependencies, public declarations, fields,
interfaces and function source spans/branch nodes. Branch nodes include closures;
they are not cyclomatic complexity. Tests and `cmd/` inbound edges are excluded.

Для запуска используйте:

```sh
go run ./tools/architecture-audit > /tmp/meetspace-architecture.json
```

Тестовые шаблоны и скрипт их запуска удалены. Утилита инвентаризации
сохранена для анализа структуры исходного кода.
