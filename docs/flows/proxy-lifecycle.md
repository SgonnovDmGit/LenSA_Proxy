# Proxy lifecycle flow

**Статус:** planned для v0.1.0; документ станет описанием current behavior после реализации и проверки соответствующего шага.

## Участники

- **Пользователь** — выбирает LAN-интерфейс, порт и опциональную Basic auth.
- **Windows UI** — отображает snapshot и отправляет команды Start/Stop.
- **Application service** — сериализует команды и управляет state machine.
- **Proxy server** — listener, source ACL, HTTP/CONNECT engine и registry соединений.
- **Target policy** — DNS resolution, проверка адресов и dial только к проверенному literal IP.

## Запуск

```text
Пользователь нажимает «Запустить»
  -> UI читает и передаёт config
  -> application: Stopped/Error -> Starting
  -> domain validation
     -> ошибка: Error + безопасное сообщение
  -> создание listener строго на selectedIPv4:port
     -> bind error: Error + безопасное сообщение
  -> запуск HTTP server
  -> application: Starting -> Running
  -> UI блокирует настройки и показывает фактический IP:PORT
```

Повторный Start в `Starting`, `Running` или `Stopping` отклоняется без создания второго listener.

## Входящее соединение

```text
TCP Accept завершён
  -> source IP входит в CIDR выбранного интерфейса?
     -> нет: connection немедленно закрывается
  -> достигнут лимит 128 connections?
     -> да: новое connection не обслуживается до освобождения capacity
  -> connection регистрируется
  -> refcount source IP увеличивается
  -> HTTP server читает request headers
```

UI показывает число уникальных source IP, refcount которых больше нуля. При закрытии последнего connection соответствующий IP удаляется из tracker.

## HTTP request

```text
Request не CONNECT
  -> absolute-form URL и scheme=http валидны?
  -> при включённой auth Proxy-Authorization корректен?
     -> нет: 407 Proxy Authentication Required
  -> hop-by-hop headers удаляются
  -> target host разрешается один раз
  -> special/private/local addresses отбрасываются
     -> нет разрешённых адресов: запрос отклоняется
  -> dial выполняется к проверенному literal IP
  -> request body передаётся потоково
  -> response headers очищаются
  -> response body передаётся потоково; SSE flush не ждёт закрытия body
```

`Proxy-Authorization`, credentials, request body и полный URL не записываются в log или файл.

## HTTPS CONNECT

```text
Request CONNECT
  -> при включённой auth credentials корректны?
     -> нет: 407 Proxy Authentication Required
  -> authority содержит destination port 443?
     -> нет: CONNECT отклоняется
  -> target host разрешается один раз
  -> special/private/local addresses отбрасываются
     -> нет разрешённых адресов: CONNECT отклоняется
  -> TCP dial к проверенному literal IP
  -> 200 Connection established
  -> двунаправленный raw byte tunnel без TLS termination и MITM
```

Client и target sockets остаются в registry до закрытия tunnel или команды Stop.

## Остановка

```text
Пользователь нажимает «Остановить»
  -> application: Running -> Stopping
  -> listener перестаёт принимать новые connections
  -> HTTP server начинает bounded shutdown
  -> registry закрывает оставшиеся connections, включая hijacked CONNECT
  -> idle upstream connections закрываются
  -> tracker очищается
  -> port освобождён
  -> application: Stopping -> Stopped
  -> UI разблокирует настройки
```

Stop в `Stopped` идемпотентен. Повторный Start после полного Stop использует новый server instance и может занять тот же адрес.

## Закрытие окна

- В `Stopped` или `Error` окно закрывается сразу.
- В `Starting`, `Running` или `Stopping` UI запрашивает подтверждение.
- После подтверждения выполняется тот же bounded Stop; process завершается только после cleanup.

## Ошибки

- Bind/start/stop failure изменяет lifecycle state и отображается в UI безопасным текстом.
- Ошибка отдельного DNS lookup, dial или upstream response возвращается только этому клиенту и не переводит работающий server из `Running` в `Error`.
- Технические ошибки не должны содержать credentials, body, token или полный URL.

## Ручная проверка

1. Start переводит UI через `Starting` в `Running` и показывает фактический адрес.
2. HTTP, HTTPS CONNECT и SSE работают с другого устройства выбранной подсети.
3. Запрещённый source и private target не обслуживаются.
4. Несколько connections одного source IP считаются одним клиентом.
5. Stop во время активного CONNECT закрывает tunnel, освобождает порт и позволяет повторный Start.
6. Закрытие окна в `Running` требует подтверждения и не оставляет process/listener.
