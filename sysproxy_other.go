//go:build !darwin

package main

// На других ОС системный прокси пока не настраиваем — используйте vpn env.
func setSystemProxy(socks, http int) error { return nil }
func clearSystemProxy() error              { return nil }
func ourProxySet(int) bool                 { return false }
func systemProxyStatus() string {
	return "не поддерживается на этой ОС (используйте vpn env)"
}

func autostartEnabled() bool { return false }
func autostartOn() error     { return errNotMac }
func autostartOff() error    { return errNotMac }

var errNotMac = errorString("автозапуск пока есть только на macOS")

type errorString string

func (e errorString) Error() string { return string(e) }
