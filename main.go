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
  vpn add <ссылка>       добавить сервер вручную
  vpn list               список серверов
  vpn use N              выбрать сервер
  vpn ping [--real]      пинг серверов
  vpn best               выбрать самый быстрый (реальный пинг)
  vpn test               проверить текущее подключение

  vpn mode global|ru     всё через VPN / российское напрямую
  vpn direct add|rm|list <домен|IP|CIDR>   исключения (идут мимо VPN)
  vpn sysproxy on|off    трогать ли системный прокси macOS
  vpn env [--off]        прокси для терминала: eval "$(vpn env)"

  vpn setup              скачать/обновить ядро Xray
  vpn config             показать конфиг Xray
  vpn log                последние строки лога
  vpn version

SSH (порт 22) и локальные сети всегда идут напрямую.`

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
		say(help)
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
			say("Предупреждение: " + err.Error())
		}
		say("VPN выключен.")
	case "toggle":
		if running, _ := isRunning(); running {
			vpnOff(st)
			say("VPN выключен.")
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
		say("Готово.")
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
	default:
		die(fmt.Errorf("неизвестная команда %q, см. vpn help", cmd))
	}
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
	say(fmt.Sprintf("Подписка обновлена: %d серверов.", n))
	if len(skipped) > 0 {
		say(fmt.Sprintf("Пропущено %d ссылок: %s", len(skipped), skipped[0]))
	}
}

func cmdOn(st *State) {
	if err := vpnOn(st, say); err != nil {
		die(err)
	}
	p, _ := st.Current()
	say(fmt.Sprintf("VPN включён: %s", p.Name))
	say(fmt.Sprintf("SOCKS 127.0.0.1:%d · HTTP 127.0.0.1:%d · SSH идёт напрямую", st.SocksPort, st.HTTPPort))
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
		say("Серверов нет. Добавьте подписку: vpn sub <url>")
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

func fmtBytes(n int64) string {
	const gb = 1 << 30
	return fmt.Sprintf("%.1f ГБ", float64(n)/gb)
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
		parts = append(parts, fmt.Sprintf("трафик %s из %s", fmtBytes(used), fmtBytes(si.Total)))
	} else if used > 0 {
		parts = append(parts, "трафик "+fmtBytes(used))
	}
	if si.Expire > 0 {
		t := time.Unix(si.Expire, 0)
		days := int(time.Until(t).Hours() / 24)
		parts = append(parts, fmt.Sprintf("до %s (%d дн.)", t.Format("02.01.2006"), days))
	}
	return strings.Join(parts, " · ")
}

func printStatus(st *State) {
	running, pid := isRunning()
	if running {
		say(fmt.Sprintf("● VPN включён (xray pid %d)", pid))
	} else {
		say("○ VPN выключен")
	}
	if p, ok := st.Current(); ok {
		say(fmt.Sprintf("Сервер:   %s (%s, %s)", p.Name, p.Summary(), p.Endpoint()))
	} else {
		say("Сервер:   не выбран")
	}
	say(fmt.Sprintf("Порты:    SOCKS %d · HTTP %d", st.SocksPort, st.HTTPPort))
	say("Режим:    " + st.Mode)
	sp := systemProxyStatus()
	if st.NoSysProxy {
		sp += " (vpn его не трогает)"
	}
	say("Сис. прокси: " + sp)
	if l := subLine(st); l != "" {
		say("Подписка: " + l)
	}
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
	say(fmt.Sprintf("Задержка: %d мс", d.Milliseconds()))
	pu, _ := url.Parse(proxy)
	cl := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}, Timeout: 8 * time.Second}
	if resp, err := cl.Get("https://api.ipify.org"); err == nil {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 100))
		resp.Body.Close()
		say("Внешний IP: " + strings.TrimSpace(string(b)))
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
