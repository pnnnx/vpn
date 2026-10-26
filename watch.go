package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const failsToSwitch = 3 // ≈1,5 минуты без ответа — переключаемся

var watchEvery = 30 * time.Second

func init() { // для тестов: VPN_WATCH_INTERVAL=2s
	if d, err := time.ParseDuration(os.Getenv("VPN_WATCH_INTERVAL")); err == nil && d > 0 {
		watchEvery = d
	}
}

func watchLogPath() string { return filepath.Join(homeDir(), "watch.log") }

func wlog(format string, a ...any) {
	fmt.Printf("%s %s\n", time.Now().Format("02.01 15:04:05"), fmt.Sprintf(format, a...))
}

// runWatch — фоновый сторож (его запускает launchd через vpn autostart on):
//   - после перезагрузки поднимает VPN, если он был включён, или снимает «висящий» прокси;
//   - перезапускает xray, если тот упал;
//   - если сервер перестал отвечать, переключается на самый быстрый из подписки.
func runWatch() {
	wlog("vpn watch запущен (pid %d)", os.Getpid())
	fails := 0
	for first := true; ; first = false {
		if !first {
			time.Sleep(watchEvery)
		}
		st, err := loadState()
		if err != nil {
			wlog("не читается состояние: %v", err)
			continue
		}
		if subUpdateDue(st) {
			if msg, err := autoUpdateSub(st); err != nil {
				wlog("автообновление подписки не удалось: %v", err)
			} else {
				wlog("%s", msg)
			}
		}
		running, _ := isRunning()
		if !st.Enabled {
			fails = 0
			if first && !running && ourProxySet(st.HTTPPort) {
				clearSystemProxy()
				wlog("VPN выключен, а системный прокси остался — снял")
			}
			continue
		}
		if !running && st.TUN && os.Geteuid() != 0 {
			if first {
				wlog("TUN-режим: сторож не может поднять VPN без root — запусти sudo vpn on")
			}
			continue
		}
		if !running {
			wlog("xray не запущен — включаю")
			if err := vpnOn(st, func(s string) { wlog("%s", s) }); err != nil {
				wlog("не удалось включить: %v", err)
			}
			continue
		}
		if _, err := httpDelay(fmt.Sprintf("http://127.0.0.1:%d", st.HTTPPort), 10*time.Second); err == nil {
			if fails > 0 {
				wlog("связь восстановилась")
			}
			fails = 0
			continue
		}
		fails++
		wlog("нет ответа через VPN (%d из %d)", fails, failsToSwitch)
		if fails < failsToSwitch || st.NoAutoSwitch || len(st.All()) < 2 {
			continue
		}
		if !online() {
			wlog("нет интернета вообще — сервер не меняю")
			continue
		}
		fails = 0
		switchToBest(st)
	}
}

// online — есть ли интернет напрямую (чтобы не «переключаться» при отключённом Wi-Fi).
func online() bool {
	for _, addr := range []string{"1.1.1.1:443", "8.8.8.8:443", "77.88.8.8:443"} {
		if c, err := dialTimeout(addr, 3*time.Second); err == nil {
			c.Close()
			return true
		}
	}
	return false
}

func switchToBest(st *State) {
	cur := st.Selected
	res, err := realPing(st, st.All(), func(string) {})
	if err != nil {
		wlog("пинг не удался: %v", err)
		return
	}
	best := -1
	for i, r := range res {
		if i != cur && r.Delay > 0 && (best < 0 || r.Delay < res[best].Delay) {
			best = i
		}
	}
	if best < 0 {
		wlog("другие серверы тоже не отвечают")
		return
	}
	st.Selected = best
	if err := vpnOn(st, func(string) {}); err != nil {
		wlog("переключение не удалось: %v", err)
		return
	}
	wlog("переключился на [%d] %s (%d мс)", best+1, st.All()[best].Name, res[best].Delay.Milliseconds())
}
