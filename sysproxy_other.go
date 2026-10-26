//go:build !darwin

package main

// На других ОС системный прокси пока не настраиваем — используйте vpn env.
func setSystemProxy(socks, http int) error { return nil }
func clearSystemProxy() error              { return nil }
func systemProxyStatus() string {
	return "не поддерживается на этой ОС (используйте vpn env)"
}
