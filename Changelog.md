# Changelog

Все существенные изменения проекта документируются в этом файле.

Формат основан на Keep a Changelog, версионирование — SemVer.

## [Unreleased]

### Added

- Создан первоначальный scaffold репозитория.
- Зафиксированы назначение, архитектурные границы и целевые сборки.
- Реализован стандартный HTTP forward proxy и HTTPS `CONNECT` без MITM.
- Добавлены source subnet ACL, public-only target policy и rebinding-safe DNS dialer.
- Добавлена опциональная Basic-аутентификация без сохранения credentials.
- Реализован bounded lifecycle listener, учёт клиентов и закрытие hijacked tunnels при Stop.
- Добавлен нативный вертикальный Win32 UI на Windigo.
- Добавлены unit/integration-тесты HTTP, SSE, TLS, auth, ACL и Stop/Restart.
- Добавлены modern amd64 и legacy Windows 386 build scripts с icon/manifest resources.
- Настроены modern verification и legacy artifact jobs в CI.
- Добавлена tag-triggered публикация GitHub Release с двумя executable и SHA-256 checksums.

### Security

- Listener привязывается только к выбранному частному IPv4-адресу.
- Private, loopback, link-local, multicast, reserved и local-host targets блокируются.
- `Proxy-Authorization` и hop-by-hop headers удаляются перед upstream.
- DNS result проверяется до dial, повторный независимый lookup не выполняется.

### Changed

- MVP сокращён до одного русского системного UI без тем, persistence, tray и firewall automation.
