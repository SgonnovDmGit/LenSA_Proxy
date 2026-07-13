# LenSA Proxy

Лёгкий portable HTTP/HTTPS forward proxy для локальной сети. Приложение запускает proxy на выбранном LAN-интерфейсе, при необходимости включает логин и пароль и показывает адрес для `LenSA_Query` или другого HTTP proxy-aware клиента.

## Статус

MVP v0.1.0 реализован и проходит unit/integration-тесты, modern Windows smoke и legacy x86 runtime smoke на текущей Windows x64. Для завершающих ручных проверок публикуется pre-release `v0.1.0-rc.2`; стабильный `v0.1.0` остаётся заблокирован до smoke в `LenSA_Query` и на Windows 7/8.1 либо Windows Server/RDP.

## Скачать

Готовые portable-сборки публикуются на странице [GitHub Releases](https://github.com/SgonnovDmGit/LenSA_Proxy/releases). Для текущей ручной проверки выберите pre-release `v0.1.0-rc.2` и скачайте подходящий executable:

- `LenSA_Proxy_windows_amd64.exe` — современная Windows x64;
- `LenSA_Proxy_windows_legacy_386.exe` — legacy Windows x86/x64;
- `SHA256SUMS.txt` — контрольные суммы обеих сборок.

Executable пока не подписаны коммерческим сертификатом, поэтому Windows SmartScreen может показать предупреждение о неизвестном издателе. Перед запуском сверьте файл с опубликованной SHA-256:

```powershell
Get-Content .\SHA256SUMS.txt
Get-FileHash .\LenSA_Proxy_windows_amd64.exe -Algorithm SHA256
```

После успешных smoke результаты фиксируются в release-документации, `Changelog.md` закрывается как `0.1.0`, а annotated tag `v0.1.0` на `main` запускает тот же workflow уже как стабильный GitHub Release.

## Возможности v0.1.0

- стандартный HTTP forward proxy;
- HTTPS через `CONNECT` без MITM и подмены сертификатов;
- выбор конкретного RFC1918 IPv4-интерфейса и порта;
- доступ только из подсети выбранного интерфейса;
- только публичные internet targets с защитой от DNS rebinding;
- `CONNECT` только к стандартному HTTPS-порту `443`;
- опциональная HTTP Basic-аутентификация;
- генерация, раскрытие и копирование временных credentials без сохранения на диск;
- отдельное копирование host и port в формате настроек `LenSA_Query`;
- нативное русское Win32-окно с Start/Stop, адресом и числом активных клиентов;
- modern Windows x64 и отдельная legacy Windows x86 сборки;
- один `.exe` без WebView2, .NET, OpenGL, CGO и внешнего runtime.

## Не входит в v0.1.0

- темы и локализация;
- сохранение настроек и пароля;
- tray, автозапуск и Windows Service;
- автоматическое изменение Windows Firewall;
- SOCKS, VPN, TUN/TAP и transparent proxy;
- HTTPS MITM, сертификаты, фильтрация и traffic history;
- IPv6 listener и системная настройка proxy Windows.

## Использование

1. Запустите подходящий `.exe` без установки.
2. Выберите LAN-интерфейс и порт, по умолчанию `8080`.
3. При необходимости включите авторизацию и введите login/password либо нажмите **Сгенерировать**.
4. Нажмите **Запустить**.
5. Для `LenSA_Query` скопируйте **Хост** без `http://`, **Порт**, login и password отдельными кнопками; режим proxy в 1С — **Настройка**.
6. Для другого proxy-aware клиента используйте те же host/port и при необходимости Basic credentials.
7. Если Windows Firewall запросит разрешение, разрешайте доступ только для доверенной частной сети. Приложение само firewall rules не создаёт.

Настройки и credentials существуют только в памяти процесса и сбрасываются после закрытия.

### Проверка через curl

```powershell
curl.exe -x http://192.168.1.42:8080 http://example.com
curl.exe -x http://192.168.1.42:8080 https://example.com
curl.exe -x http://192.168.1.42:8080 -U user:password https://example.com
```

Замените адрес proxy на значение из окна приложения.

## Артефакты

| Файл | Назначение |
|---|---|
| `LenSA_Proxy_windows_amd64.exe` | Windows 10/11 и современные Windows Server x64 |
| `LenSA_Proxy_windows_legacy_386.exe` | Windows 7/8.1 и legacy Windows Server/RDP, x86/x64 |

Legacy binary собирается `go-legacy-win7 v1.26.4-1`. Совместимость считается подтверждённой только после runtime smoke на целевой ОС.

## Сборка из исходников

Требуется официальный Go 1.26.1 или совместимый более новый toolchain.

```powershell
.\scripts\build-modern.ps1 -Version 0.1.0
.\scripts\build-legacy.ps1 -Version 0.1.0
```

Modern script использует установленный официальный Go. Legacy script скачивает зафиксированный Windows amd64 archive `go-legacy-win7 v1.26.4-1`, проверяет SHA-256 и cross-компилирует x86 artifact. Toolchain cache и binaries находятся в ignored `build/`/`dist/`.

Оба script:

- генерируют Win32 resources через `github.com/akavel/rsrc v0.10.2`;
- встраивают LP icon, Common Controls v6 и system-DPI manifest;
- используют `CGO_ENABLED=0`, `-trimpath` и Windows GUI subsystem;
- печатают SHA-256 готового `.exe`.

## Разработка и проверки

```powershell
go test ./...
go vet ./...
go test -race ./...
.\scripts\build-modern.ps1 -OutputDirectory build\verify-modern
.\scripts\build-legacy.ps1 -OutputDirectory build\verify-legacy
```

Integration suite поднимает локальные HTTP/TLS/TCP backends и проверяет HTTP body/headers, Basic auth, SSE streaming, неизменный target TLS certificate, private-target blocking, `CONNECT`, Stop и повторный bind порта без внешнего internet dependency.

## Стек

- **Go 1.23 language baseline** — domain, application и network core;
- **goproxy v1.8.4** — HTTP forward proxy и `CONNECT` engine;
- **Windigo v0.2.6** — pure-Go Win32 UI;
- **go-legacy-win7 v1.26.4-1** — отдельный legacy build toolchain;
- **rsrc v0.10.2** — compile-time icon/manifest resources.

## Архитектура

Слои: `presentation → application → domain ← infrastructure`.

- `cmd/lensa-proxy/` — composition root;
- `internal/presentation/windows/` — нативный Windows UI;
- `internal/application/` — Start/Stop state machine и snapshots;
- `internal/domain/proxy/` — конфигурация, состояния и инварианты;
- `internal/infrastructure/network/` — interfaces, ACL, safe resolver/dialer, listener и proxy engine;
- `tests/integration/` — end-to-end HTTP/SSE/TLS/CONNECT lifecycle tests.

UI не содержит сетевой логики. Windows-specific код изолирован в presentation/composition root; proxy core не зависит от Windigo.

## Безопасность

- listener никогда не использует `0.0.0.0`;
- source IP берётся из TCP connection и проверяется по выбранному CIDR;
- private, loopback, link-local, multicast, CGNAT, documentation и local-host targets блокируются;
- DNS разрешается один раз, dial выполняется к проверенному literal IP;
- credentials сравниваются через fixed-size SHA-256 digest и constant-time comparison;
- `Proxy-Authorization` и hop-by-hop headers не уходят upstream;
- request body, credentials, token и полный URL не логируются;
- Stop закрывает обычные и hijacked `CONNECT` connections;
- HTTPS payload остаётся end-to-end encrypted между клиентом и target.

## Связанные репозитории

| Репозиторий | Роль | Описание |
|---|---|---|
| `LenSA_Query` | клиент | Внешняя обработка 1С с host/port/credentials HTTP proxy |
| `swancode_server` | server | Основной upstream LenSA |
| `rdp-ghost` | UI reference | Референс compact portable Windows utility |

## Документация

- `Changelog.md` — история изменений;
- `docs/flows/proxy-lifecycle.md` — current lifecycle HTTP/CONNECT;
- `docs/mockups/v0.1-ui.html` — утверждённый UI mockup;
- `docs/specs/` — локальные phase specs;
- `docs/todo.md` — локальный roadmap.
