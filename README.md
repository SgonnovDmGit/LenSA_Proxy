# LenSA Proxy

Лёгкий portable HTTP/HTTPS forward proxy для локальной сети. Приложение запускает proxy на выбранном LAN-интерфейсе, при необходимости включает логин и пароль и показывает адрес для `LenSA_Query` или другого HTTP proxy-aware клиента.

## Статус

`v0.1.0` — первый стабильный релиз. Он проходит unit/integration-тесты, modern Windows smoke и интеграционный smoke с `LenSA_Query`, включая Basic-аутентификацию и длинный SSE-ответ. Legacy x86 executable собирается воспроизводимо и запускается на текущей Windows x64, но совместимость с Windows 7/8.1 и legacy Windows Server/RDP остаётся неподтверждённой до целевого runtime smoke, запланированного на `v0.3.0`.

## Скачать

Готовые portable-сборки публикуются на странице [GitHub Releases](https://github.com/SgonnovDmGit/LenSA_Proxy/releases). Выберите стабильный `v0.1.0` и скачайте подходящий executable:

- `LenSA_Proxy_windows_amd64.exe` — современная Windows x64;
- `LenSA_Proxy_windows_legacy_386.exe` — неподтверждённая compatibility-сборка для legacy Windows x86/x64;
- `LenSA_Proxy_linux_amd64.tar.gz` / `LenSA_Proxy_linux_arm64.tar.gz` — Linux;
- `LenSA_Proxy_darwin_amd64.tar.gz` / `LenSA_Proxy_darwin_arm64.tar.gz` — macOS Intel / Apple Silicon;
- `SHA256SUMS.txt` — контрольные суммы всех артефактов.

Executable пока не подписаны коммерческим сертификатом, поэтому Windows SmartScreen может показать предупреждение о неизвестном издателе. Перед запуском сверьте файл с опубликованной SHA-256:

```powershell
Get-Content .\SHA256SUMS.txt
Get-FileHash .\LenSA_Proxy_windows_amd64.exe -Algorithm SHA256
```

## Возможности v0.1.0

- стандартный HTTP forward proxy;
- HTTPS через `CONNECT` без MITM и подмены сертификатов;
- выбор конкретного RFC1918 IPv4-интерфейса и порта;
- доступ только из подсети выбранного интерфейса;
- только публичные internet targets с защитой от DNS rebinding;
- `CONNECT` только к стандартному HTTPS-порту `443`;
- опциональная HTTP Basic-аутентификация;
- генерация, раскрытие и копирование временных credentials без сохранения на диск;
- вычисляемый host без URL scheme и копирование port прямо из поля настройки для `LenSA_Query`;
- нативное русское Win32-окно с Start/Stop, адресом и числом активных клиентов;
- modern Windows x64 и отдельная legacy Windows x86 сборки;
- один `.exe` без WebView2, .NET, OpenGL, CGO и внешнего runtime.

## Возможности v0.2.0

- portable persistence не-secret настроек: interface, port и auth toggle сохраняются в `LenSA_Proxy.ini` рядом с `.exe` при закрытии и восстанавливаются при старте;
- пароль не сохраняется и не читается;
- tolerant fallback на defaults при отсутствии, повреждении или неполном `.ini`, а также при ненайденном имени интерфейса;
- сохранение настроек — best-effort: ошибки записи игнорируются, приложение не падает;
- light/dark/system темы: переключатель в header окна, выбор сохраняется в `.ini` (`theme = light|dark|system`);
- dark palette через hardcoded hex colors, light palette — системные цвета Windows;
- системная тема: запрос Windows 10+ dark preference через registry; Windows 7 — всегда light;
- High Contrast detection: при активном HC theme toggle отключён, используются системные цвета.

## Возможности v1.0.0

- кроссплатформенность: нативное окно на Linux и macOS (Fyne) с паритетом формы Windows-версии — тот же набор полей, действий и русский текст интерфейса;
- безопасное хранение credentials: при включённой авторизации пара логин/пароль сохраняется в системное хранилище ОС после успешного запуска proxy — Windows Credential Manager, macOS Keychain, Linux Secret Service;
- при следующем старте пара восстанавливается в поля формы автоматически;
- при выключенной авторизации сохранённая запись удаляется из хранилища после успешного Start;
- ошибка Start не трогает сохранённые credentials; недоступность системного хранилища не мешает работе proxy — окно показывает предупреждение, пара живёт только в памяти сессии;
- `LenSA_Proxy.ini` по-прежнему хранит только не-secret настройки (интерфейс, порт, auth toggle, тема) и работает на всех платформах рядом с бинарём;
- release-артефакты собираются нативно на каждой платформе (CGO для Fyne).

## Не входит в v0.1.0

- локализация;
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
5. Для `LenSA_Query` скопируйте **Хост** без `http://`, port из основного поля, login и password соседними кнопками; режим proxy в 1С — **Настройка**.
6. Для другого proxy-aware клиента используйте те же host/port и при необходимости Basic credentials.
7. Если Windows Firewall запросит разрешение, разрешайте доступ только для доверенной частной сети. Приложение само firewall rules не создаёт.
8. Переключайте тему (Светлая/Тёмная/Системная) кнопками в header окна. Выбор сохраняется в `.ini` и восстанавливается при следующем запуске. При активном Windows High Contrast theme toggle отключён.

При закрытии приложение сохраняет выбранный интерфейс, порт, состояние авторизации и тему в `LenSA_Proxy.ini` рядом с executable. При следующем запуске настройки восстанавливаются автоматически. Пара логин/пароль сохраняется отдельно — в системном хранилище ОС, только после успешного запуска proxy с включённой авторизацией; при отключённой авторизации запись удаляется. Если `.ini` отсутствует, повреждён или содержит несуществующее имя интерфейса, приложение тихо откатывается к defaults.

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
| `LenSA_Proxy_windows_legacy_386.exe` | Неподтверждённая compatibility-сборка для Windows 7/8.1 и legacy Windows Server/RDP, x86/x64 |
| `LenSA_Proxy_linux_amd64.tar.gz` | Desktop Linux x64 (X11/Wayland, glibc) |
| `LenSA_Proxy_linux_arm64.tar.gz` | Desktop Linux ARM64 |
| `LenSA_Proxy_darwin_amd64.tar.gz` | macOS Intel |
| `LenSA_Proxy_darwin_arm64.tar.gz` | macOS Apple Silicon |

Legacy binary собирается `go-legacy-win7 v1.26.4-1`. Совместимость считается неподтверждённой до runtime smoke на целевой ОС; проверка запланирована на `v0.3.0`.

## Сборка из исходников

Требуется официальный Go 1.26.1 или совместимый более новый toolchain.

```powershell
.\scripts\build-modern.ps1 -Version 1.0.0
.\scripts\build-legacy.ps1 -Version 1.0.0
```

Linux/macOS (нативная сборка, CGO обязателен):

```bash
# Linux: dev-заголовки Fyne
sudo apt-get install -y gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev

./scripts/build-unix.sh 1.0.0
```

`build-unix.sh` собирает `LenSA_Proxy_<os>_<arch>.tar.gz` в `dist/`. Fyne требует CGO и нативные GUI-заголовки, поэтому linux/arm64 собирается на arm64-раннере или arm64-хосте, darwin amd64/arm64 — на соответствующих macOS-хостах.

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
- **Windigo v0.2.6** — pure-Go Win32 UI (Windows);
- **Fyne v2.8.2** — нативный UI для Linux/macOS (CGO);
- **go-keyring v0.2.8** — системное хранилище credentials (Credential Manager / Keychain / Secret Service);
- **go-legacy-win7 v1.26.4-1** — отдельный legacy build toolchain;
- **rsrc v0.10.2** — compile-time icon/manifest resources.

## Архитектура

Слои: `presentation → application → domain ← infrastructure`.

- `cmd/lensa-proxy/` — composition roots: `main.go` (Windows) и `main_fyne.go` (Linux/macOS) через build-теги;
- `internal/presentation/windows/` — нативный Windows UI (Windigo);
- `internal/presentation/fyneui/` — нативный UI для Linux/macOS (Fyne, `linux || darwin || fyneui`);
- `internal/presentation/form/` — общая view-model формы и парсинг для обоих UI;
- `internal/application/` — Start/Stop state machine, snapshots и lifecycle credentials;
- `internal/domain/proxy/` — конфигурация, состояния и инварианты;
- `internal/infrastructure/network/` — interfaces, ACL, safe resolver/dialer, listener и proxy engine;
- `internal/infrastructure/keyring/` — системное хранилище credentials;
- `internal/infrastructure/storage/` — не-secret настройки в `LenSA_Proxy.ini`;
- `tests/integration/` — end-to-end HTTP/SSE/TLS/CONNECT lifecycle tests.

UI не содержит сетевой логики. Платформенный код изолирован в presentation/composition roots; proxy core не зависит от Windigo и Fyne. View-тесты fyneui запускаются без CGO: `go test -tags fyneuitest ./internal/presentation/fyneui/` (in-memory driver).

## Безопасность

- listener никогда не использует `0.0.0.0`;
- source IP берётся из TCP connection и проверяется по выбранному CIDR;
- private, loopback, link-local, multicast, CGNAT, documentation и local-host targets блокируются;
- DNS разрешается один раз, dial выполняется к проверенному literal IP;
- credentials сравниваются через fixed-size SHA-256 digest и constant-time comparison;
- сохранённые credentials живут только в системном хранилище ОС, никогда — в `.ini`, логах и release-документации;
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
