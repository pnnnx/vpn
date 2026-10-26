package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func configPath() string { return filepath.Join(homeDir(), "config.json") }
func logPath() string    { return filepath.Join(homeDir(), "xray.log") }
func pidPath() string    { return filepath.Join(homeDir(), "xray.pid") }

func readPID() int {
	data, err := os.ReadFile(pidPath())
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

// isXray проверяет, что под этим PID живёт именно xray (PID мог переиспользоваться).
func isXray(pid int) bool {
	if pid <= 0 || syscall.Kill(pid, 0) != nil {
		return false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	return err == nil && strings.Contains(filepath.Base(strings.TrimSpace(string(out))), "xray")
}

func isRunning() (bool, int) {
	pid := readPID()
	return isXray(pid), pid
}

func tailLog(n int) string {
	f, err := os.Open(logPath())
	if err != nil {
		return ""
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n")
}

func ensureXray(st *State, out func(string)) (string, error) {
	if x := findXray(); x != "" {
		return x, nil
	}
	out("Ядро Xray не найдено — скачиваю…")
	if err := setupXray(st, out); err != nil {
		return "", err
	}
	return findXray(), nil
}

// startXray пишет конфиг, проверяет его и запускает xray в фоне.
func startXray(st *State, out func(string)) error {
	p, ok := st.Current()
	if !ok {
		return fmt.Errorf("сервер не выбран: сначала vpn sub <url> или vpn use N")
	}
	xray, err := ensureXray(st, out)
	if err != nil {
		return err
	}
	assets := assetDir(xray)
	cfg, err := buildConfig(st, p, assets != "")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(homeDir(), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(configPath(), cfg, 0o600); err != nil {
		return err
	}
	env := os.Environ()
	if assets != "" {
		env = append(env, "XRAY_LOCATION_ASSET="+assets)
	}

	test := exec.Command(xray, "run", "-test", "-c", configPath())
	test.Env = env
	if b, err := test.CombinedOutput(); err != nil {
		return fmt.Errorf("xray отклонил конфиг:\n%s", strings.TrimSpace(string(b)))
	}

	if running, _ := isRunning(); running {
		if err := stopXray(); err != nil {
			return err
		}
	}
	for _, port := range []int{st.SocksPort, st.HTTPPort} {
		if portOpen(port) {
			return fmt.Errorf("порт %d уже занят другой программой (выключи другой VPN-клиент)", port)
		}
	}

	logf, err := os.Create(logPath())
	if err != nil {
		return err
	}
	cmd := exec.Command(xray, "run", "-c", configPath())
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // не умирает вместе с терминалом
	if err := cmd.Start(); err != nil {
		logf.Close()
		return err
	}
	logf.Close()
	pid := cmd.Process.Pid
	os.WriteFile(pidPath(), []byte(strconv.Itoa(pid)), 0o600)
	cmd.Process.Release()

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if portOpen(st.SocksPort) {
			return nil
		}
		if syscall.Kill(pid, 0) != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stopXray()
	return fmt.Errorf("xray не запустился. Лог:\n%s", tailLog(15))
}

func stopXray() error {
	pid := readPID()
	if isXray(pid) {
		if err := syscall.Kill(pid, syscall.SIGTERM); err == syscall.EPERM {
			// xray запущен через sudo (TUN) — обычный пользователь его не остановит
			return fmt.Errorf("%s", L("xray запущен от root (TUN): выключи через sudo vpn off",
				"xray runs as root (TUN): stop it with sudo vpn off"))
		}
		for i := 0; i < 20 && isXray(pid); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		if isXray(pid) {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	os.Remove(pidPath())
	return nil
}

func portOpen(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// vpnOn / vpnOff — общая логика для CLI и TUI.
// Сохраняют желаемое состояние, чтобы vpn watch знал, что делать после перезагрузки.
func vpnOn(st *State, out func(string)) error {
	if st.TUN && os.Geteuid() != 0 {
		return fmt.Errorf("%s", L("TUN-режиму нужен root: запусти sudo vpn on (или выключи: vpn tun off)",
			"TUN mode needs root: run sudo vpn on (or disable it: vpn tun off)"))
	}
	defer fixOwner()
	if err := startXray(st, out); err != nil {
		return err
	}
	st.Enabled = true
	st.Save()
	if st.TUN { // весь трафик и так идёт через TUN — системный прокси не нужен
		if ourProxySet(st.HTTPPort) {
			clearSystemProxy()
		}
		return nil
	}
	// Если прокси уже наш (например, после перезагрузки) — не трогаем, лишний раз не спрашиваем пароль.
	if !st.NoSysProxy && !ourProxySet(st.HTTPPort) {
		if err := setSystemProxy(st.SocksPort, st.HTTPPort); err != nil {
			return fmt.Errorf("xray запущен, но системный прокси не включился: %w", err)
		}
	}
	return nil
}

func vpnOff(st *State) error {
	if err := stopXray(); err != nil {
		return err
	}
	st.Enabled = false
	st.Save()
	return clearSystemProxy() // снимаем всегда, даже если xray уже мёртв
}
