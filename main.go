package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

var version = "0.1.0-dev"

const help = `vpn — консольный клиент Xray для macOS

  vpn                    открыть TUI
  vpn on [N]             включить (сервер N, иначе текущий)
  vpn off                выключить
  vpn toggle             переключить
  vpn status             состояние

  vpn sub <url>          задать подписку и обновить
  vpn update             обновить подписку
  vpn autoupdate on|off  обновлять подписку раз в сутки (по умолчанию да)
  vpn add <ссылка>       добавить сервер вручную
  vpn list               список серверов
  vpn use N              выбрать сервер
  vpn ping [--real]      пинг серверов
  vpn best               выбрать самый быстрый (реальный пинг)
  vpn test               проверить текущее подключение

  vpn mode global|ru     всё через VPN / российское напрямую
  vpn direct add|rm|list <домен|IP|CIDR>   исключения (идут мимо VPN)
  vpn sysproxy on|off    трогать ли системный прокси macOS
  vpn tun on|off         экспериментальный TUN-режим для всех программ (нужен sudo)
  vpn env [--off]        прокси для терминала: eval "$(vpn env)"

  vpn autostart on|off   сторож в фоне: поднимает VPN после перезагрузки,
                         перезапускает xray, переключает сервер при падении
  vpn autoswitch on|off  переключать ли сервер, если текущий не отвечает

  vpn setup              скачать/обновить ядро Xray
  vpn config             показать конфиг Xray
  vpn log                последние строки лога
  vpn lang ru|en         язык интерфейса
  vpn version

SSH (порт 22) и локальные сети всегда идут напрямую.`

const helpEN = `vpn — a terminal Xray client for macOS

  vpn                    open the TUI
  vpn on [N]             connect (server N, or the current one)
  vpn off                disconnect
  vpn toggle             toggle
  vpn status             show status

  vpn sub <url>          set the subscription and update it
  vpn update             update the subscription
  vpn autoupdate on|off  update the subscription once a day (default: on)
  vpn add <link>         add a server manually
  vpn list               list servers
  vpn use N              select a server
  vpn ping [--real]      ping all servers
  vpn best               pick the fastest (real ping)
  vpn test               check the current connection

  vpn mode global|ru     everything via VPN / Russian sites direct
  vpn direct add|rm|list <domain|IP|CIDR>   exclusions (bypass the VPN)
  vpn sysproxy on|off    whether to manage the macOS system proxy
  vpn tun on|off         experimental TUN mode for all apps (needs sudo)
  vpn env [--off]        proxy for the terminal: eval "$(vpn env)"

  vpn autostart on|off   background watcher: restores the VPN after reboot,
                         restarts xray, switches server when it fails
  vpn autoswitch on|off  whether to switch server when the current one fails

  vpn setup              download/update the Xray core
  vpn config             print the Xray config
  vpn log                show recent log lines
  vpn lang ru|en         interface language
  vpn version

SSH (port 22) and local networks always go direct.`

func say(s string) { fmt.Println(s) }

func die(err error) {
	fmt.Fprintln(os.Stderr, "Ошибка:", err)
	os.Exit(1)
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func main() {
	st, err := loadState()
	if err != nil {
		die(fmt.Errorf("не читается %s: %w", statePath(), err))
	}
	langPref = st.Lang
	if len(os.Args) < 2 {
		if isTerminal() {
			if err := runTUI(st); err != nil {
				die(err)
			}
			return
		}
		printStatus(st)
		return
	}
	cmd, args := os.Args[1], os.Args[2:]
	save := func() {
		if err := st.Save(); err != nil {
			die(err)
		}
	}
	arg := func(i int, what string) string {
		if len(args) <= i {
			die(fmt.Errorf("не хватает аргумента: %s", what))
		}
		return args[i]
	}

	switch cmd {
	case "help", "-h", "--help":
		say(L(help, helpEN))
	case "version", "--version":
		say("vpn " + version)
	case "setup":
		if err := setupXray(st, say); err != nil {
			die(err)
		}
	case "sub":
		st.SubURL = strings.TrimSpace(arg(0, "url подписки"))
		cmdUpdate(st)
		save()
	case "update":
		cmdUpdate(st)
		save()
	case "add":
		p, err := ParseLink(arg(0, "ссылка"))
		if err != nil {
			die(err)
		}
		st.Manual = append(st.Manual, p)
		if st.Selected < 0 {
			st.Selected = len(st.All()) - 1
		}
		save()
		say("Добавлен: " + p.Name)
	case "list", "ls":
		printList(st, nil)
	case "use":
		selectN(st, arg(0, "номер сервера"))
		save()
		p, _ := st.Current()
		say("Выбран: " + p.Name)
		if running, _ := isRunning(); running {
			cmdOn(st)
		}
	case "ping":
		real := len(args) > 0 && (args[0] == "--real" || args[0] == "-r")
		res := doPing(st, real)
		printList(st, res)
		printFailures(st, res)
	case "best":
		res := doPing(st, true)
		best := -1
		for i, r := range res {
			if r.Delay > 0 && (best < 0 || r.Delay < res[best].Delay) {
				best = i
			}
		}
		if best < 0 {
			die(fmt.Errorf("ни один сервер не ответил"))
		}
		st.Selected = best
		save()
		say(fmt.Sprintf("Лучший: [%d] %s — %s", best+1, st.All()[best].Name, fmtDelay(res[best])))
		if running, _ := isRunning(); running {
			cmdOn(st)
		}
	case "on", "connect", "up":
		if len(args) > 0 {
			selectN(st, args[0])
			save()
		}
		cmdOn(st)
	case "off", "disconnect", "down":
		if err := vpnOff(st); err != nil {
			if running, _ := isRunning(); running {
				die(err)
			}
			say(L("Предупреждение: ", "Warning: ") + err.Error())
		}
		say(L("VPN выключен.", "VPN is off."))
	case "toggle":
		if running, _ := isRunning(); running {
			if err := vpnOff(st); err != nil {
				die(err)
			}
			say(L("VPN выключен.", "VPN is off."))
		} else {
			cmdOn(st)
		}
	case "status", "st":
		printStatus(st)
	case "test":
		cmdTest(st)
	case "mode":
		m := arg(0, "global или ru")
		if m != "global" && m != "ru" {
			die(fmt.Errorf("режим: global или ru"))
		}
		st.Mode = m
		save()
		say("Режим: " + m)
		restartIfRunning(st)
	case "direct":
		cmdDirect(st, args)
		save()
	case "sysproxy":
		switch arg(0, "on или off") {
		case "on":
			st.NoSysProxy = false
		case "off":
			st.NoSysProxy = true
			clearSystemProxy()
		default:
			die(fmt.Errorf("sysproxy on|off"))
		}
		save()
		say(L("Готово.", "Done."))
	case "env":
		if len(args) > 0 && args[0] == "--off" {
			say("unset http_proxy https_proxy all_proxy no_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY")
			return
		}
		h := fmt.Sprintf("http://127.0.0.1:%d", st.HTTPPort)
		s := fmt.Sprintf("socks5://127.0.0.1:%d", st.SocksPort)
		say(fmt.Sprintf("export http_proxy=%s https_proxy=%s all_proxy=%s no_proxy=localhost,127.0.0.1,::1", h, h, s))
		say(fmt.Sprintf("export HTTP_PROXY=%s HTTPS_PROXY=%s ALL_PROXY=%s NO_PROXY=localhost,127.0.0.1,::1", h, h, s))
	case "config":
		p, ok := st.Current()
		if !ok {
			die(fmt.Errorf("сервер не выбран"))
		}
		cfg, err := buildConfig(st, p, assetDir(findXray()) != "")
		if err != nil {
			die(err)
		}
		say(string(cfg))
	case "log":
		say(tailLog(40))
		if b, err := os.ReadFile(watchLogPath()); err == nil {
			say("--- watch ---")
			say(lastLines(string(b), 15))
		}
	case "autoupdate":
		switch arg(0, "on | off") {
		case "on":
			st.NoSubAutoUpdate = false
		case "off":
			st.NoSubAutoUpdate = true
		default:
			die(fmt.Errorf("vpn autoupdate on|off"))
		}
		save()
		say(L("Готово.", "Done."))
	case "lang":
		l := arg(0, "ru | en")
		if l != "ru" && l != "en" && l != "auto" {
			die(fmt.Errorf("vpn lang ru|en|auto"))
		}
		if l == "auto" {
			l = ""
		}
		st.Lang = l
		save()
		say(map[string]string{"ru": "Язык: русский", "en": "Language: English", "": "Language: auto / Язык: как в системе"}[l])
	case "tun":
		switch arg(0, "on | off") {
		case "on":
			st.TUN = true
			save()
			say(L("TUN-режим включён (экспериментально): через VPN пойдут все программы, а не только те, что уважают системный прокси.",
				"TUN mode enabled (experimental): all apps go through the VPN, not only those that respect the system proxy."))
			say(L("Нужен root: включай через  sudo vpn on  и выключай через  sudo vpn off.",
				"Needs root: connect with  sudo vpn on  and disconnect with  sudo vpn off."))
			say(L("SSH, локальные сети и исключения по-прежнему идут напрямую. IPv6 в TUN не заворачивается.",
				"SSH, local networks and exclusions still go direct. IPv6 is not routed through TUN."))
		case "off":
			st.TUN = false
			save()
			say(L("TUN-режим выключен. Если VPN сейчас включён в TUN — sudo vpn on, чтобы переключиться на прокси.",
				"TUN mode disabled. If the VPN is running in TUN — run sudo vpn on to switch to proxy mode."))
		default:
			die(fmt.Errorf("vpn tun on|off"))
		}
	case "watch":
		runWatch()
	case "autostart":
		switch arg(0, "on или off") {
		case "on":
			if err := autostartOn(); err != nil {
				die(err)
			}
			say("Автозапуск включён: сторож работает в фоне и стартует при входе в систему.")
			say("Его журнал: vpn log")
		case "off":
			if err := autostartOff(); err != nil {
				die(err)
			}
			say("Автозапуск выключен.")
		default:
			die(fmt.Errorf("vpn autostart on|off"))
		}
	case "autoswitch":
		switch arg(0, "on или off") {
		case "on":
			st.NoAutoSwitch = false
		case "off":
			st.NoAutoSwitch = true
		default:
			die(fmt.Errorf("vpn autoswitch on|off"))
		}
		save()
		say(L("Готово.", "Done."))
	default:
		die(fmt.Errorf("неизвестная команда %q, см. vpn help", cmd))
	}
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func selectN(st *State, s string) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > len(st.All()) {
		die(fmt.Errorf("нет сервера с номером %s (всего %d)", s, len(st.All())))
	}
	st.Selected = n - 1
}

func cmdUpdate(st *State) {
	n, skipped, err := updateSub(st)
	if err != nil {
		die(err)
	}
	say(fmt.Sprintf(L("Подписка обновлена: %d серверов.", "Subscription updated: %d servers."), n))
	if len(skipped) > 0 {
		say(fmt.Sprintf("Пропущено %d ссылок: %s", len(skipped), skipped[0]))
	}
}

func cmdOn(st *State) {
	if err := vpnOn(st, say); err != nil {
		die(err)
	}
	p, _ := st.Current()
	say(fmt.Sprintf(L("VPN включён: %s", "VPN is on: %s"), p.Name))
	say(fmt.Sprintf(L("SOCKS 127.0.0.1:%d · HTTP 127.0.0.1:%d · SSH идёт напрямую", "SOCKS 127.0.0.1:%d · HTTP 127.0.0.1:%d · SSH goes direct"), st.SocksPort, st.HTTPPort))
}

func restartIfRunning(st *State) {
	if running, _ := isRunning(); running {
		cmdOn(st)
	}
}

func doPing(st *State, real bool) []PingResult {
	all := st.All()
	if len(all) == 0 {
		die(fmt.Errorf("список пуст: vpn sub <url>"))
	}
	if !real {
		return tcpPing(all)
	}
	res, err := realPing(st, all, say)
	if err != nil {
		die(err)
	}
	return res
}

func printList(st *State, res []PingResult) {
	all := st.All()
	if len(all) == 0 {
		say(L("Серверов нет. Добавьте подписку: vpn sub <url>", "No servers. Add a subscription: vpn sub <url>"))
		return
	}
	order := make([]int, len(all))
	for i := range order {
		order[i] = i
	}
	if res != nil { // с пингом — сортируем, недоступные в конце
		sort.SliceStable(order, func(a, b int) bool {
			ra, rb := res[order[a]].Delay, res[order[b]].Delay
			if ra < 0 {
				return false
			}
			if rb < 0 {
				return true
			}
			return ra < rb
		})
	}
	for _, i := range order {
		p := all[i]
		mark := "  "
		if i == st.Selected {
			mark = "▶ "
		}
		delay := ""
		if res != nil {
			delay = fmt.Sprintf("%8s  ", fmtDelay(res[i]))
		}
		say(fmt.Sprintf("%s%3d  %s%-22s %s", mark, i+1, delay, p.Summary(), p.Name))
	}
}

func printFailures(st *State, res []PingResult) {
	all := st.All()
	first := true
	for i, r := range res {
		if r.Delay >= 0 || r.Err == nil {
			continue
		}
		if first {
			say(L("\nНе ответили:", "\nNot responding:"))
			first = false
		}
		say(fmt.Sprintf("  %3d  %s — %s", i+1, all[i].Name, shortErr(r.Err)))
	}
}

// shortErr — человеческая причина вместо длинной ошибки Go.
func shortErr(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "Client.Timeout"), strings.Contains(s, "timeout"), strings.Contains(s, "deadline"):
		return L("таймаут", "timeout")
	case strings.Contains(s, "connection refused"):
		return L("соединение отклонено", "connection refused")
	case strings.Contains(s, "EOF"), strings.Contains(s, "reset"):
		return L("сервер оборвал соединение", "server closed the connection")
	case strings.Contains(s, "no such host"):
		return L("домен не найден", "host not found")
	}
	return short(s)
}

func fmtBytes(n int64) string {
	const gb = 1 << 30
	return fmt.Sprintf(L("%.1f ГБ", "%.1f GB"), float64(n)/gb)
}

func subLine(st *State) string {
	si := st.SubInfo
	if si == nil {
		return ""
	}
	var parts []string
	if si.Title != "" {
		parts = append(parts, si.Title)
	}
	used := si.Upload + si.Download
	if si.Total > 0 {
		parts = append(parts, fmt.Sprintf(L("трафик %s из %s", "traffic %s of %s"), fmtBytes(used), fmtBytes(si.Total)))
	} else if used > 0 {
		parts = append(parts, L("трафик ", "traffic ")+fmtBytes(used))
	}
	if si.Expire > 0 {
		t := time.Unix(si.Expire, 0)
		days := int(time.Until(t).Hours() / 24)
		parts = append(parts, fmt.Sprintf(L("до %s (%d дн.)", "until %s (%d days)"), t.Format("02.01.2006"), days))
	}
	return strings.Join(parts, " · ")
}

func printStatus(st *State) {
	running, pid := isRunning()
	if running {
		say(fmt.Sprintf(L("● VPN включён (xray pid %d)", "● VPN is on (xray pid %d)"), pid))
	} else {
		say(L("○ VPN выключен", "○ VPN is off"))
	}
	if p, ok := st.Current(); ok {
		say(fmt.Sprintf(L("Сервер:   %s (%s, %s)", "Server:   %s (%s, %s)"), p.Name, p.Summary(), p.Endpoint()))
	} else {
		say(L("Сервер:   не выбран", "Server:   not selected"))
	}
	say(fmt.Sprintf(L("Порты:    SOCKS %d · HTTP %d", "Ports:    SOCKS %d · HTTP %d"), st.SocksPort, st.HTTPPort))
	mode := st.Mode
	if st.TUN {
		mode += " · TUN"
	}
	say(L("Режим:    ", "Mode:     ") + mode)
	sp := map[string]string{"включён": L("включён", "on"), "выключен": L("выключен", "off")}[systemProxyStatus()]
	if sp == "" {
		sp = systemProxyStatus()
	}
	if st.NoSysProxy {
		sp += " (vpn его не трогает)"
	}
	say(L("Сис. прокси: ", "System proxy: ") + sp)
	if l := subLine(st); l != "" {
		say(L("Подписка: ", "Subscription: ") + l)
	}
	auto := L("выключен (vpn autostart on)", "off (vpn autostart on)")
	if autostartEnabled() {
		auto = L("включён", "on")
		if !st.NoAutoSwitch {
			auto += L(", с автопереключением", ", with failover")
		}
	}
	say(L("Автозапуск: ", "Autostart: ") + auto)
	if running && systemProxyStatus() == "выключен" && !st.NoSysProxy {
		say("Внимание: xray работает, но системный прокси выключен — vpn on")
	}
	if !running && systemProxyStatus() == "включён" {
		say("Внимание: системный прокси включён, а xray нет — интернет может не работать. vpn off")
	}
}

func cmdTest(st *State) {
	if running, _ := isRunning(); !running {
		die(fmt.Errorf("VPN выключен"))
	}
	proxy := fmt.Sprintf("http://127.0.0.1:%d", st.HTTPPort)
	d, err := httpDelay(proxy, 8*time.Second)
	if err != nil {
		die(fmt.Errorf("через VPN не открывается %s: %w", testURL, err))
	}
	say(fmt.Sprintf(L("Задержка: %d мс", "Latency: %d ms"), d.Milliseconds()))
	pu, _ := url.Parse(proxy)
	cl := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}, Timeout: 8 * time.Second}
	if resp, err := cl.Get("https://api.ipify.org"); err == nil {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 100))
		resp.Body.Close()
		say(L("Внешний IP: ", "External IP: ") + strings.TrimSpace(string(b)))
	}
}

func cmdDirect(st *State, args []string) {
	if len(args) == 0 || args[0] == "list" {
		if len(st.Direct) == 0 {
			say("Своих исключений нет. SSH и локальные сети идут напрямую всегда.")
		}
		for _, d := range st.Direct {
			say("  " + d)
		}
		return
	}
	if len(args) < 2 {
		die(fmt.Errorf("vpn direct add|rm <домен|IP|CIDR>"))
	}
	switch args[0] {
	case "add":
		st.Direct = append(st.Direct, args[1:]...)
	case "rm", "del":
		var keep []string
		for _, d := range st.Direct {
			drop := false
			for _, r := range args[1:] {
				if d == r {
					drop = true
				}
			}
			if !drop {
				keep = append(keep, d)
			}
		}
		st.Direct = keep
	default:
		die(fmt.Errorf("vpn direct add|rm|list"))
	}
	say("Исключения: " + strings.Join(st.Direct, ", "))
	restartIfRunning(st)
}
