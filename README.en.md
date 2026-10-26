<div align="center">

# vpn

**A terminal Xray client for macOS: TUI, subscriptions, Reality, and SSH that keeps working.**

[![CI](https://github.com/pnnnx/vpn/actions/workflows/ci.yml/badge.svg)](https://github.com/pnnnx/vpn/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/pnnnx/vpn?sort=semver)](https://github.com/pnnnx/vpn/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/pnnnx/vpn)](go.mod)
[![Platform](https://img.shields.io/badge/platform-macOS-lightgrey?logo=apple)](#install)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**English** · [Русский](README.md)

</div>

```
 ● ВКЛ  Helsinki  режим global
 SOCKS :10808 · HTTP :10809 · SSH напрямую · My VPN · трафик 42.0 ГБ из 200 ГБ

    1  vless+reality        Amsterdam                                182 мс
▶   2  vless+reality        Helsinki                                  96 мс
    3  vless+xhttp+reality  Frankfurt                                241 мс
    4  vless+grpc+reality   Bucharest                                512 мс
    5  trojan+tls           New York                                      —
    6  shadowsocks          London                                   377 мс
```

Type `vpn` to get a list of servers from your subscription. `Space` turns the VPN on and off. For scripts there are plain commands: `vpn on`, `vpn off`, `vpn status`.

> The interface is currently in Russian. Translations are welcome.

## Why

GUI clients for VLESS are heavy. Many of them run in TUN mode and send **all** traffic through the VPN, SSH included. Then `ssh` to your own servers breaks, because the VPN server blocks outbound port 22.

`vpn` runs [Xray-core](https://github.com/XTLS/Xray-core) as a local proxy and sets it as the macOS system proxy. **SSH, private networks and your exclusions always go direct.**

## Features

- 🔗 **Provider subscriptions** (Marzban, Remnawave, 3x-ui, …), with remaining traffic and expiry date.
- 🔐 **Protocols:** `vless://` (TCP, WS, gRPC, XHTTP, HTTPUpgrade; TLS and **Reality**), `vmess://`, `trojan://`, `ss://`.
- 🖥 **TUI** built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
- ⚡ **Real latency test:** a request through every server in parallel, not just a TCP connect.
- 🧭 **Routing:** `global` or `ru` mode (Russian sites go direct), plus custom domain, IP and CIDR exclusions.
- 🛡 **SSH always bypasses the VPN:** port 22 and private networks go direct.
- 🔁 **Autostart and failover:** a background watcher (launchd) brings the VPN back after a reboot, restarts a crashed xray and switches to the fastest server when the current one stops responding.
- 📈 **Live ↑/↓ speed** and session traffic in the TUI.
- 🍎 **macOS system proxy** is toggled automatically; `vpn env` covers terminal tools.
- 📦 **Self-installing core:** downloads the latest Xray-core and verifies its SHA-256.
- 🪶 A single binary with no daemon and no GUI. The VPN keeps running after you close the terminal.

## Install

**From a release** (Apple Silicon; use `amd64` for Intel):

```sh
mkdir -p ~/.local/bin
curl -fsSL https://github.com/pnnnx/vpn/releases/latest/download/vpn_darwin_arm64.tar.gz | tar xz -C ~/.local/bin vpn
```

**With Homebrew:**

```sh
brew install --cask pnnnx/tap/vpn
```

**With Go** 1.24+:

```sh
go install github.com/pnnnx/vpn@latest
```

**From source:**

```sh
git clone https://github.com/pnnnx/vpn && cd vpn && make build
```

## Quick start

```sh
vpn sub 'https://your-subscription-url'
vpn          # TUI: P — real ping, b — fastest, Enter — connect
```

## Keys

| Key | Action |
|---|---|
| `↑` `↓` / `j` `k` | move |
| `Enter` | connect to the highlighted server |
| `Space` / `o` | on / off |
| `p` / `P` | TCP ping / real ping |
| `b` | jump to the fastest server |
| `u` | update the subscription |
| `m` | switch mode `global` ↔ `ru` |
| `d` | exclusions that bypass the VPN (`a` add, `x` remove) |
| `i` | server details (secrets masked) |
| `l` | xray and watcher log |
| `s` | set the subscription URL |
| `q` | quit (the VPN keeps running) |

## Commands

`vpn on [N]`, `off`, `toggle`, `status`, `autostart on|off`, `autoswitch on|off`, `sub <url>`, `update`, `add <link>`, `list`, `use N`, `ping [--real]`, `best`, `test`, `mode global|ru`, `direct add|rm|list <domain|ip|cidr>`, `sysproxy on|off`, `env [--off]`, `setup`, `config`, `log`, `version`. Run `vpn help` for details.

## How it works

`vpn` generates an Xray config, starts `xray` as a detached background process with SOCKS5 on `127.0.0.1:10808` and HTTP on `127.0.0.1:10809`, and points the macOS system proxy at it with `networksetup`. Terminal `ssh` ignores the system proxy. In addition, the first routing rule sends port 22 and all private ranges to `direct`, so SSH stays direct even when traffic is forced through Xray.

State lives in `~/.config/vpn/`. `state.json` is stored with mode `0600` because it contains keys. Set `VPN_HOME` to use a different folder.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md).

> ⚠️ Never post real share links or subscription URLs in issues: they contain your access keys.

## License

[MIT](LICENSE) © pnnnx
