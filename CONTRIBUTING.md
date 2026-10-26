# Участие в разработке

Спасибо, что хочешь помочь! Issue, идеи и pull request'ы приветствуются.

## Перед тем как писать issue

- **Удали из ссылок и логов всё секретное:** UUID, пароли, `pbk`/`sid`, адреса серверов, URL подписки. Вместо них подставь `11111111-2222-3333-4444-555555555555` и `example.com`.
- Приложи вывод `vpn version`, `vpn status` и версию macOS.
- Для ошибок xray пригодится `vpn log`, тоже без адресов.

## Сборка и тесты

Нужен Go 1.24+.

```sh
make build       # ./vpn
make test        # unit-тесты
make test-xray   # + проверка сгенерированных конфигов настоящим xray (скачает ядро)
make vet         # go vet для текущей ОС и для darwin
make dist        # бинарники darwin arm64/amd64 в dist/
```

Пробный запуск, не трогая свои настройки:

```sh
VPN_HOME=/tmp/vpn-dev ./vpn
```

## Структура

| Файл | Что делает |
|---|---|
| `link.go` | парсинг ссылок `vless://`, `vmess://`, `trojan://`, `ss://` |
| `sub.go` | скачивание и разбор подписок |
| `state.go` | `~/.config/vpn/state.json` |
| `xray.go` | генерация конфига Xray и правила маршрутизации |
| `runner.go` | запуск и остановка xray, `vpnOn` / `vpnOff` |
| `sysproxy_darwin.go` | системный прокси macOS (`networksetup`) |
| `ping.go` | TCP- и реальный пинг |
| `setup.go` | установка Xray-core с проверкой SHA-256 |
| `tui.go` | интерфейс на Bubble Tea |
| `main.go` | CLI |

Логика (парсер, конфиг, раннер) не зависит от TUI. CLI и TUI вызывают одни и те же функции.

## Как добавить протокол или транспорт

1. Разбор ссылки — в `ParseLink` (`link.go`), плюс тест в `link_test.go` с фейковыми данными.
2. Outbound и `streamSettings` — в `outbound` / `streamSettings` (`xray.go`).
3. Добавь ссылку в `allLinks()`, чтобы её покрыл `make test-xray`.

## Стиль

- `gofmt` и `go vet` должны быть чистыми — это проверяет CI.
- Сообщения для пользователя пишутся на русском.
- Коммиты в стиле [Conventional Commits](https://www.conventionalcommits.org/ru/): `feat:`, `fix:`, `docs:`, `test:`, `ci:`, `chore:`.
- Один PR — одна логическая правка. Опиши, что и зачем, и как проверял.
