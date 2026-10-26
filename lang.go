package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// Язык интерфейса: VPN_LANG=ru|en → настройка `vpn lang` → язык macOS → LANG.
// Сообщения об ошибках пока только на русском.
var (
	langOnce sync.Once
	langEN   bool
	langPref string // из state.json, выставляется в main
)

func detectLang() bool {
	for _, v := range []string{os.Getenv("VPN_LANG"), langPref} {
		switch strings.ToLower(v) {
		case "en":
			return true
		case "ru":
			return false
		}
	}
	if runtime.GOOS == "darwin" {
		// Язык системы macOS надёжнее LANG: у многих русских пользователей LANG=en_US.
		if out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
			for _, f := range strings.FieldsFunc(string(out), func(r rune) bool { return strings.ContainsRune("(),\" \n\t", r) }) {
				return !strings.HasPrefix(strings.ToLower(f), "ru")
			}
		}
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return !strings.HasPrefix(strings.ToLower(v), "ru")
		}
	}
	return false
}

func isEN() bool {
	langOnce.Do(func() { langEN = detectLang() })
	return langEN
}

// L выбирает строку по языку интерфейса.
func L(ru, en string) string {
	if isEN() {
		return en
	}
	return ru
}
