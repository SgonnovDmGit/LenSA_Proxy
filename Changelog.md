# Changelog

Все существенные изменения проекта документируются в этом файле.

Формат основан на Keep a Changelog, версионирование — SemVer.

## [Unreleased]

### Added

- Кроссплатформенная поддержка: нативное окно на Linux и macOS (Fyne v2.8) с полным parity к Windows-форме — выбор интерфейса, порт, auth, генерация/показ/копирование credentials, вычисляемый host, темы, Start/Stop, статус и счётчик клиентов.
- Безопасное хранение credentials в системном хранилище ОС (`go-keyring` v0.2.8): Windows Credential Manager, macOS Keychain, Linux Secret Service. Пара логин/пароль сохраняется одной записью после успешного Start с включённой авторизацией и восстанавливается в поля формы при следующем запуске.
- Удаление сохранённой credential-записи после успешного Start без авторизации.
- Индикатор недоступности системного хранилища в обоих UI; store-ошибки не блокируют запуск proxy.
- Общий платформенно-нейтральный пакет `internal/presentation/form`: view-model, парсинг формы, preview-адрес и безопасные тексты ошибок, переиспользуемые обоими UI.
- `scripts/build-unix.sh` — нативная сборка `LenSA_Proxy_<os>_<arch>.tar.gz` с ldflags version/commit.
- CI `verify-unix` matrix job: linux amd64, linux arm64 (нативный `ubuntu-24.04-arm`), darwin amd64, darwin arm64 — fmt/vet/test + сборка артефакта.
- Release workflow реструктурирован в multi-job: `validate` → `build-windows` + `build-unix` (нативные раннеры) → `publish` с шестью артефактами и общим `SHA256SUMS.txt`.
- `docs/flows/credential-store.md` — lifecycle credential-хранилища.

### Changed

- `internal/presentation/windows/view.go` разделён: платформенно-нейтральная логика переехала в `internal/presentation/form` с экспортированными типами; поведение Windows UI не изменилось.
- `main.go` ограничен `windows && !fyneui`; добавлен `main_fyne.go` для `linux || darwin || fyneui`.
- Проверка errno «порт занят» разнесена по per-OS файлам (`errno_windows.go` / `errno_unix.go`), shared `service.go` полностью переносим.
- `LenSA_Proxy.ini` теперь работает и на nix — рядом с executable, как на Windows.

### Security

- Credentials никогда не записываются в `.ini`, логи и release-документацию; plaintext-fallback при недоступном хранилище отсутствует.
- Ошибка Start не изменяет сохранённые credentials: save/delete выполняются только после успешного `server.Start()`.

## [0.2.0] — 2026-07-25

### Added

- Portable persistence не-secret настроек: interface, port и auth toggle сохраняются в `LenSA_Proxy.ini` рядом с `.exe` при закрытии и восстанавливаются при старте.
- Пароль не сохраняется и не читается.
- Tolerant fallback на defaults при отсутствии, повреждении или неполном `.ini`, а также при ненайденном имени интерфейса.
- Сохранение настроек — best-effort: ошибки записи игнорируются.
- Light/dark/system темы: переключатель в header окна, выбор сохраняется в `.ini` (`theme = light|dark|system`).
- Dark palette через hardcoded hex colors; light palette — системные цвета Windows.
- Системная тема: запрос Windows 10+ dark preference через registry; Windows 7 — всегда light.
- High Contrast detection: при активном HC theme toggle отключён, используются системные цвета.
- Пересоздание brushes/pens и repaint при смене темы.

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
