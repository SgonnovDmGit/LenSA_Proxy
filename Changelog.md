# Changelog

Все существенные изменения проекта документируются в этом файле.

Формат основан на Keep a Changelog, версионирование — SemVer.

## [0.1.0] — 2026-07-14

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
- Добавлены cryptographic generator, reveal и отдельное копирование login/password.
- Добавлены вычисляемый копируемый host и кнопка копирования рядом с полем port для настроек `LenSA_Query`.

### Security

- Listener привязывается только к выбранному частному IPv4-адресу.
- Private, loopback, link-local, multicast, reserved и local-host targets блокируются.
- `Proxy-Authorization` и hop-by-hop headers удаляются перед upstream.
- DNS result проверяется до dial, повторный независимый lookup не выполняется.

### Changed

- MVP сокращён до одного русского системного UI без тем, persistence, tray и firewall automation.
- Credentials остаются selectable read-only во время работы proxy; изменяемые сетевые настройки по-прежнему заблокированы.
- Интеграционный smoke с `LenSA_Query`, включая Basic-аутентификацию и длинный SSE-ответ, подтверждён на modern Windows-сборке.
- Legacy 386 executable публикуется как неподтверждённая compatibility-сборка; целевой runtime smoke на Windows 7/8.1 или Windows Server/RDP перенесён в `v0.3.0`.

### Fixed

- Конфликт занятого порта на Windows корректно распознаётся как `WSAEADDRINUSE` и показывает понятную ошибку.
- Ответ `407` на HTTPS `CONNECT` теперь явно закрывает соединение, позволяя клиентам повторить запрос с Basic-аутентификацией на новом сокете.
