# Credential store flow

**Статус:** current behavior v1.0.0; lifecycle покрыт unit-тестами `internal/application` и `internal/infrastructure/keyring`.

## Участники

- **Application service** — владеет lifecycle: загрузка при старте приложения, save/delete после успешного Start.
- **CredentialStore** (`internal/application`) — интерфейс потребителя: `Load`, `Save`, `Delete` над `proxy.Credentials`.
- **Keyring store** (`internal/infrastructure/keyring`) — системное хранилище ОС: Windows Credential Manager, macOS Keychain, Linux Secret Service. Одна запись `service=LenSA_Proxy`, `account=proxy-credentials`, значение `username\npassword`.
- **Presentation (windows/fyneui)** — читает восстановленную пару через `LoadedCredentials`, показывает предупреждение при `CredentialStoreAvailable() == false`.

## Загрузка при старте приложения

```text
application.NewService(WithCredentialStore(store))
  -> store.Load()
     -> записи нет:      пара отсутствует, store считается доступным
     -> запись повреждена (нет разделителя, пустой username): считается отсутствующей
     -> ошибка backend:  CredentialStoreAvailable = false, пара отсутствует
  -> пара хранится в памяти сервиса (не в Snapshot, не в PersistedSettings)
  -> UI при applyInitialSettings подставляет логин/пароль в поля
  -> UI при недоступном store показывает warning: пароль не сохранится
```

## Сохранение / удаление при Start

```text
UI -> service.Start(config)
  -> config.Validate / server.Start()
     -> ошибка: stored credentials НЕ изменяются (ни save, ни delete)
  -> Start успешен
     -> config.AuthEnabled:  store.Save("username\npassword")
     -> !config.AuthEnabled: store.Delete()
     -> ошибка store: лог/индикатор, Start НЕ отменяется
```

Сохранение только после успешного Start — проверка: если bind порта упал, пользователь не теряет ранее сохранённую рабочую пару.

## Инварианты

- Хранится пара целиком: логин `lensa-XXXXXX` генерируется вместе с паролем, восстановление одного пароля бесполезно.
- Никакого plaintext-fallback: ошибка store означает «не сохранять», а не «писать в файл».
- `.ini` (`LenSA_Proxy.ini`) содержит только не-secret настройки: interface, port, auth, theme.
- Ошибки Load/Save/Delete мапятся в `proxy.ErrStoreUnavailable` с сохранением причины через `%w`.
