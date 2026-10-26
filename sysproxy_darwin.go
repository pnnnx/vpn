//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

var bypass = []string{"localhost", "127.0.0.1", "::1", "*.local", "169.254/16",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

// networkServices — включённые сетевые службы (Wi-Fi, Ethernet, …).
func networkServices() ([]string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, err
	}
	var svcs []string
	for i, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" || strings.HasPrefix(line, "*") { // первая строка — пояснение, * — выключена
			continue
		}
		svcs = append(svcs, line)
	}
	return svcs, nil
}

func ns(args ...string) error {
	if b, err := exec.Command("networksetup", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("networksetup %s: %s", args[0], strings.TrimSpace(string(b)))
	}
	return nil
}

func setSystemProxy(socks, http int) error {
	svcs, err := networkServices()
	if err != nil {
		return err
	}
	s, h := strconv.Itoa(socks), strconv.Itoa(http)
	for _, svc := range svcs {
		for _, args := range [][]string{
			{"-setwebproxy", svc, "127.0.0.1", h},
			{"-setsecurewebproxy", svc, "127.0.0.1", h},
			{"-setsocksfirewallproxy", svc, "127.0.0.1", s},
			append([]string{"-setproxybypassdomains", svc}, bypass...),
		} {
			if err := ns(args...); err != nil {
				return err
			}
		}
	}
	return nil
}

func clearSystemProxy() error {
	svcs, err := networkServices()
	if err != nil {
		return err
	}
	var firstErr error
	for _, svc := range svcs {
		for _, flag := range []string{"-setwebproxystate", "-setsecurewebproxystate", "-setsocksfirewallproxystate"} {
			if err := ns(flag, svc, "off"); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// ourProxySet — системный HTTP-прокси уже указывает на наш порт.
func ourProxySet(httpPort int) bool {
	out, err := exec.Command("scutil", "--proxy").Output()
	if err != nil {
		return false
	}
	s := string(out)
	return strings.Contains(s, "HTTPEnable : 1") && strings.Contains(s, "HTTPProxy : 127.0.0.1") &&
		strings.Contains(s, "HTTPPort : "+strconv.Itoa(httpPort))
}

func systemProxyStatus() string {
	out, err := exec.Command("scutil", "--proxy").Output()
	if err != nil {
		return "неизвестно"
	}
	s := string(out)
	if strings.Contains(s, "HTTPEnable : 1") || strings.Contains(s, "SOCKSEnable : 1") {
		return "включён"
	}
	return "выключен"
}
