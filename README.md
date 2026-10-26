<div align="center">

# vpn

**Консольный клиент Xray для macOS: TUI, подписки, Reality, и SSH, который не ломается.**

[![CI](https://github.com/pnnnx/vpn/actions/workflows/ci.yml/badge.svg)](https://github.com/pnnnx/vpn/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/pnnnx/vpn?sort=semver)](https://github.com/pnnnx/vpn/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/pnnnx/vpn)](go.mod)
[![Platform](https://img.shields.io/badge/platform-macOS-lightgrey?logo=apple)](#установка)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

[English](README.en.md) · **Русский**

</div>

```
 ● ВКЛ  Helsinki  режим global · автозапуск
 ↑ 48 КБ/с  ↓ 2.4 МБ/с  · за сессию 318.6 МБ · My VPN · трафик 42.0 ГБ из 200 ГБ

    1  vless+reality        Amsterdam                                182 мс
▶   2  vless+reality        Helsinki                                  96 мс
    3  vless+xhttp+reality  Frankfurt                                241 мс
    4  vless+grpc+reality   Bucharest                                512 мс
    5  trojan+tls           New York                                      —
    6  shadowsocks          London                                   377 мс

 Enter подключить · Space вкл/выкл · p/P пинг · b лучший · d исключения · l лог · q выход
```

Пишешь `vpn` — открывается список серверов из подписки. `Space` включает и выключает VPN. Для скриптов есть обычные команды: `vpn on`, `vpn off`, `vpn status`.

## Зачем

GUI-клиенты для VLESS тяжёлые. Многие в TUN-режиме заворачивают через VPN **весь** трафик, включая SSH, и `ssh` на свои серверы перестаёт работать: VPN-сервер режет исходящий порт 22.

`vpn` работает иначе. Он запускает [Xray-core](https://github.com/XTLS/Xray-core) как локальный прокси и включает его системным прокси macOS. **SSH, локальные сети и твои исключения всегда идут напрямую.**

## Возможности

- 🔗 **Подписки от хостера** (Marzban, Remnawave, 3x-ui и др.). Показывает остаток трафика и срок.
- 🔐 **Протоколы:** `vless://` (TCP, WS, gRPC, XHTTP, HTTPUpgrade; TLS и **Reality**), `vmess://`, `trojan://`, `ss://`.
- 🖥 **TUI** на [Bubble Tea](https://github.com/charmbracelet/bubbletea): навигация, пинг с подсветкой, выбор лучшего сервера.
- ⚡ **Реальный пинг:** запрос через каждый сервер одновременно, а не просто TCP до порта.
- 🧭 **Маршрутизация:** режим `global` или `ru` (российское напрямую), свои исключения — домены, IP, подсети.
- 🛡 **SSH всегда мимо VPN:** порт 22 и приватные сети напрямую.
- 🔁 **Автозапуск и автопереключение:** фоновый сторож поднимает VPN после перезагрузки, перезапускает упавший xray и переходит на самый быстрый сервер, если текущий перестал отвечать.
- 📈 **Скорость ↑/↓** и трафик за сессию прямо в TUI.
- 🍎 **Системный прокси macOS** включается и выключается сам; `vpn env` — для терминала.
- 📦 **Ядро ставится само:** скачивается последний Xray-core с проверкой SHA-256.
- 🪶 Один бинарник, без демонов и GUI. VPN продолжает работать после закрытия терминала.

## Установка

**Из релиза** (Apple Silicon; для Intel замени `arm64` на `amd64`):

```sh
mkdir -p ~/.local/bin
curl -fsSL https://github.com/pnnnx/vpn/releases/latest/download/vpn_darwin_arm64.tar.gz | tar xz -C ~/.local/bin vpn
```

**Через Homebrew:**

```sh
brew install --cask pnnnx/tap/vpn
```

**Через Go** (1.24+):

```sh
go install github.com/pnnnx/vpn@latest
```

**Из исходников:**

```sh
git clone https://github.com/pnnnx/vpn && cd vpn
make build && mv vpn ~/.local/bin/
```

Если `~/.local/bin` ещё нет в PATH:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
```

## Быстрый старт

```sh
vpn sub 'https://ссылка-на-подписку'   # загрузить серверы
vpn                                    # TUI: P — пинг, b — лучший, Enter — подключиться
```

При первом включении `vpn` сам скачает ядро Xray, а macOS может спросить пароль — так меняется системный прокси.

## TUI

| Клавиша | Действие |
|---|---|
| `↑` `↓` / `j` `k` | навигация |
| `Enter` | подключиться к выбранному серверу |
| `Space` / `o` | включить / выключить |
| `p` / `P` | TCP-пинг / реальный пинг |
| `b` | перейти к самому быстрому |
| `u` | обновить подписку |
| `m` | режим `global` ↔ `ru` |
| `d` | исключения: что идёт мимо VPN (`a` — добавить, `x` — удалить) |
| `i` | детали сервера (ключи скрыты) |
| `l` | лог xray и сторожа |
| `s` | задать ссылку на подписку |
| `q` | выйти. **VPN продолжит работать** в фоне |

## Команды

| Команда | Что делает |
|---|---|
| `vpn` | открыть TUI |
| `vpn on [N]` · `off` · `toggle` | включить / выключить |
| `vpn status` | состояние, сервер, подписка |
| `vpn sub <url>` · `update` | задать / обновить подписку |
| `vpn add <ссылка>` | добавить сервер вручную |
| `vpn list` · `use N` | список / выбрать сервер |
| `vpn ping [--real]` · `best` | пинг / выбрать лучший |
| `vpn test` | задержка и внешний IP через VPN |
| `vpn mode global\|ru` | режим маршрутизации |
| `vpn direct add\|rm\|list <x>` | исключения: домен, IP или CIDR |
| `vpn autostart on\|off` | фоновый сторож: автозапуск, перезапуск, автопереключение |
| `vpn autoswitch on\|off` | переключать ли сервер, если текущий не отвечает |
| `vpn sysproxy on\|off` | трогать ли системный прокси |
| `vpn env [--off]` | прокси для терминала: `eval "$(vpn env)"` |
| `vpn setup` | скачать / обновить Xray |
| `vpn config` · `log` · `version` | конфиг Xray, лог, версия |

## Как это работает

```
  vpn (CLI / TUI) ──► генерирует config.json, запускает и останавливает процесс
        │
        ▼
  xray (фоновый процесс) ── SOCKS5 127.0.0.1:10808
                          └─ HTTP   127.0.0.1:10809
        ▲
  системный прокси macOS ◄── браузеры и приложения
  ssh, git по ssh, локальная сеть ──► напрямую
```

- Протоколы `vpn` сам не реализует: криптография и транспорт — это Xray-core. `vpn` собирает конфиг и управляет процессом.
- xray запускается в отдельной сессии. Закрытие терминала или выход из TUI VPN не выключает.
- Режим — **локальный прокси, не TUN**. Через VPN идут программы, которые уважают системный прокси: Safari, Chrome, Telegram и большинство приложений.

### Почему SSH работает

1. Терминальный `ssh` системный прокси не использует, поэтому идёт напрямую.
2. Даже если трафик попадёт в xray (через `vpn env` или proxychains), первое правило маршрутизации отправляет **порт 22 напрямую**. Так же напрямую идут `10/8`, `172.16/12`, `192.168/16`, `100.64/10`, `localhost` и `*.local`.

SSH на нестандартном порту:

```sh
vpn direct add 203.0.113.7           # IP сервера
vpn direct add myserver.example.com  # домен и все поддомены
```

### Режимы

- **`global`** — всё, кроме исключений, идёт через VPN.
- **`ru`** — напрямую идут ещё `.ru`, `.su` и `.рф`, а с geo-файлами (их ставит `vpn setup`) — `geosite:category-ru` и `geoip:ru`.

### Автозапуск и автопереключение

```sh
vpn autostart on
```

Ставит агент launchd (`~/Library/LaunchAgents/io.github.pnnnx.vpn.plist`), который запускает `vpn watch` при входе в систему. Сторож раз в 30 секунд:

- **после перезагрузки** поднимает VPN, если он был включён, или снимает «висящий» системный прокси, если был выключен;
- **перезапускает xray**, если тот упал;
- **переключает сервер**, если через VPN нет ответа ~1,5 минуты, а интернет напрямую есть: делает реальный пинг всех серверов и берёт самый быстрый. Отключается командой `vpn autoswitch off`.

`vpn off` сторож уважает и VPN обратно не включает. Журнал сторожа — в `vpn log` и на экране `l` в TUI. Убрать агент: `vpn autostart off`.

### Терминал

`curl`, `git`, `brew` системный прокси не видят. Для текущего окна терминала:

```sh
eval "$(vpn env)"        # включить
eval "$(vpn env --off)"  # выключить
```

## Данные

Всё хранится в `~/.config/vpn/` (переопределяется переменной `VPN_HOME`):

| Файл | Содержимое |
|---|---|
| `state.json` | подписка, серверы, настройки. Права `0600` — внутри ключи |
| `config.json` | сгенерированный конфиг Xray |
| `xray.log` · `xray.pid` | лог и PID |
| `bin/` | `xray`, `geoip.dat`, `geosite.dat` |

## Решение проблем

<details>
<summary><b>После перезагрузки пропал интернет</b></summary>

Системный прокси остался включённым, а xray после перезагрузки не запущен. Выполни `vpn on` или `vpn off`; `vpn status` предупреждает о такой ситуации. Чтобы это больше не повторялось, включи `vpn autostart on`.
</details>

<details>
<summary><b>«порт 10808 уже занят»</b></summary>

Работает другой VPN-клиент. Закрой его или поменяй `socks_port` и `http_port` в `~/.config/vpn/state.json`.
</details>

<details>
<summary><b>Подписка или ядро не скачиваются</b></summary>

Включи VPN на любом сервере и повтори: при неудаче напрямую `vpn` пробует скачать через себя. Ядро можно поставить и через `brew install xray`.
</details>

<details>
<summary><b>Сервер в пинге «не отвечает»</b></summary>

`vpn ping --real` в конце пишет причину: таймаут, обрыв соединения и т.д. Если сервер то отвечает, то нет, проблема на стороне хостера. При включённом `vpn autostart on` клиент сам уйдёт на рабочий сервер.
</details>

<details>
<summary><b>«xray отклонил конфиг»</b></summary>

Обнови ядро (`vpn setup`) и посмотри `vpn config`. Если ссылка работает в других клиентах, [открой issue](https://github.com/pnnnx/vpn/issues/new/choose), но **сначала удали из ссылки UUID, ключи и адрес сервера**.
</details>

<details>
<summary><b>Системный прокси не включается</b></summary>

`networksetup` может попросить пароль администратора. Можно не трогать системный прокси совсем: `vpn sysproxy off` и `vpn env`.
</details>

## Планы

- [x] Автозапуск, восстановление после перезагрузки и автопереключение (launchd)
- [x] Скорость ↑/↓ и трафик в TUI (Xray stats)
- [x] Экраны исключений, деталей сервера и лога в TUI
- [x] Homebrew tap
- [ ] TUN-режим для всего трафика системы
- [ ] Linux: системный прокси для GNOME/KDE
- [ ] Hysteria2 / TUIC

## Участие

Pull request'ы и issue приветствуются — см. [CONTRIBUTING.md](CONTRIBUTING.md). Об уязвимостях — [SECURITY.md](SECURITY.md).

> ⚠️ Никогда не прикладывай к issue реальные ссылки и подписки: в них ключи доступа к твоему VPN.

## Благодарности

[XTLS/Xray-core](https://github.com/XTLS/Xray-core) · [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) · [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)

## Лицензия

[MIT](LICENSE) © pnnnx
