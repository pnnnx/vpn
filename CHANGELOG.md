# Changelog

Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версии — [SemVer](https://semver.org/lang/ru/).

## [Unreleased]

## [0.3.0] — 2026-10-26

### Добавлено
- Экспериментальный TUN-режим: `vpn tun on`, затем `sudo vpn on`. Весь IPv4-трафик системы идёт через VPN, SSH и локальные сети — по-прежнему напрямую.
- Автообновление подписки раз в сутки (сторож и запуск TUI), `vpn autoupdate on|off`. Если у текущего сервера сменились параметры — переподключение.
- Английский интерфейс: язык берётся из macOS, `vpn lang ru|en|auto`, `VPN_LANG`.
- Сайт проекта на GitHub Pages с SEO-разметкой (schema.org, Open Graph, sitemap).

### Исправлено
- Homebrew cask: снятие карантина переведено на новый формат GoReleaser (`postflight_steps`).

## [0.2.0] — 2026-10-26

### Добавлено
- `vpn autostart on|off`: агент launchd с фоновым сторожем `vpn watch`. Поднимает VPN после перезагрузки, снимает «висящий» прокси, перезапускает упавший xray.
- Автопереключение на самый быстрый сервер, если текущий не отвечает ~1,5 минуты (`vpn autoswitch on|off`).
- TUI: скорость ↑/↓ и трафик за сессию (Xray stats), экраны исключений (`d`), деталей сервера (`i`) и лога (`l`). TUI подхватывает изменения от сторожа.
- `vpn ping --real` пишет причину для неответивших серверов.
- Установка через Homebrew: `brew install --cask pnnnx/tap/vpn`.

### Исправлено
- Реальный пинг: прогревочный запрос и ограничение параллельности. Цифры больше не завышены в разы.
- Скачивание ядра Xray: таймаут 10 минут вместо 20 секунд.
- `vpn on` не трогает системный прокси, если он уже наш: меньше запросов пароля.

## [0.1.0] — 2026-10-26

### Добавлено
- TUI: список серверов, вкл/выкл, TCP- и реальный пинг, выбор лучшего, режимы, ввод подписки.
- CLI: `on`, `off`, `toggle`, `status`, `sub`, `update`, `add`, `list`, `use`, `ping`, `best`, `test`, `mode`, `direct`, `sysproxy`, `env`, `setup`, `config`, `log`.
- Ссылки `vless://` (TCP/WS/gRPC/XHTTP/HTTPUpgrade, TLS, Reality), `vmess://`, `trojan://`, `ss://`.
- Подписки в base64 и текстом; трафик и срок из `Subscription-Userinfo`.
- SSH (порт 22) и приватные сети всегда идут напрямую.
- Системный прокси macOS через `networksetup`.
- Автоустановка Xray-core с проверкой SHA-256.

[Unreleased]: https://github.com/pnnnx/vpn/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/pnnnx/vpn/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/pnnnx/vpn/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/pnnnx/vpn/releases/tag/v0.1.0
