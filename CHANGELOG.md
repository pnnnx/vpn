# Changelog

Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версии — [SemVer](https://semver.org/lang/ru/).

## [Unreleased]

## [0.1.0] — 2026-10-26

### Добавлено
- TUI: список серверов, вкл/выкл, TCP- и реальный пинг, выбор лучшего, режимы, ввод подписки.
- CLI: `on`, `off`, `toggle`, `status`, `sub`, `update`, `add`, `list`, `use`, `ping`, `best`, `test`, `mode`, `direct`, `sysproxy`, `env`, `setup`, `config`, `log`.
- Ссылки `vless://` (TCP/WS/gRPC/XHTTP/HTTPUpgrade, TLS, Reality), `vmess://`, `trojan://`, `ss://`.
- Подписки в base64 и текстом; трафик и срок из `Subscription-Userinfo`.
- SSH (порт 22) и приватные сети всегда идут напрямую.
- Системный прокси macOS через `networksetup`.
- Автоустановка Xray-core с проверкой SHA-256.

[Unreleased]: https://github.com/pnnnx/vpn/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/pnnnx/vpn/releases/tag/v0.1.0
