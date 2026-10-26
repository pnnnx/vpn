package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type obj = map[string]any

// Приватные сети и локалхост всегда идут напрямую.
var privateCIDRs = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"169.254.0.0/16", "100.64.0.0/10", "::1/128", "fc00::/7", "fe80::/10",
}

func binDir() string { return filepath.Join(homeDir(), "bin") }

// findXray ищет ядро: своя папка, Homebrew, PATH.
func findXray() string {
	cands := []string{
		filepath.Join(binDir(), "xray"),
		"/opt/homebrew/bin/xray",
		"/usr/local/bin/xray",
	}
	for _, c := range cands {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("xray"); err == nil {
		return p
	}
	return ""
}

// assetDir — где лежат geoip.dat/geosite.dat (нужны только режиму ru).
func assetDir(xrayPath string) string {
	cands := []string{binDir()}
	if xrayPath != "" {
		real, _ := filepath.EvalSymlinks(xrayPath)
		cands = append(cands, filepath.Dir(xrayPath), filepath.Dir(real),
			filepath.Join(filepath.Dir(filepath.Dir(real)), "share", "xray"))
	}
	cands = append(cands, "/opt/homebrew/share/xray", "/usr/local/share/xray")
	for _, c := range cands {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "geoip.dat")); err == nil {
			if _, err := os.Stat(filepath.Join(c, "geosite.dat")); err == nil {
				return c
			}
		}
	}
	return ""
}

func outbound(p Profile, tag string) (obj, error) {
	o := obj{"tag": tag, "protocol": p.Protocol}
	switch p.Protocol {
	case "vless":
		user := obj{"id": p.ID, "encryption": first(p.Encryption, "none")}
		if p.Flow != "" {
			user["flow"] = p.Flow
		}
		o["settings"] = obj{"vnext": []obj{{"address": p.Address, "port": p.Port, "users": []obj{user}}}}
	case "vmess":
		user := obj{"id": p.ID, "security": first(p.Encryption, "auto")}
		if p.AlterID > 0 {
			user["alterId"] = p.AlterID
		}
		o["settings"] = obj{"vnext": []obj{{"address": p.Address, "port": p.Port, "users": []obj{user}}}}
	case "trojan":
		o["settings"] = obj{"servers": []obj{{"address": p.Address, "port": p.Port, "password": p.Password}}}
	case "shadowsocks":
		o["settings"] = obj{"servers": []obj{{"address": p.Address, "port": p.Port, "method": p.Method, "password": p.Password}}}
	default:
		return nil, fmt.Errorf("протокол %s не поддерживается", p.Protocol)
	}
	if p.Protocol != "shadowsocks" {
		o["streamSettings"] = streamSettings(p)
	}
	return o, nil
}

func streamSettings(p Profile) obj {
	network := first(p.Network, "tcp")
	if network == "raw" {
		network = "tcp"
	}
	if network == "h2" {
		network = "http"
	}
	ss := obj{"network": network}

	switch network {
	case "ws":
		ss["wsSettings"] = obj{"path": first(p.Path, "/"), "host": p.Host}
	case "httpupgrade":
		ss["httpupgradeSettings"] = obj{"path": first(p.Path, "/"), "host": p.Host}
	case "grpc":
		ss["grpcSettings"] = obj{"serviceName": first(p.ServiceName, p.Path), "multiMode": p.Mode == "multi"}
	case "xhttp":
		x := obj{"path": first(p.Path, "/"), "mode": first(p.Mode, "auto")}
		if p.Host != "" {
			x["host"] = p.Host
		}
		if len(p.Extra) > 0 {
			x["extra"] = p.Extra
		}
		ss["xhttpSettings"] = x
	case "kcp", "mkcp":
		ss["network"] = "kcp"
		k := obj{"header": obj{"type": first(p.HeaderType, "none")}}
		if p.Seed != "" {
			k["seed"] = p.Seed
		}
		ss["kcpSettings"] = k
	case "tcp":
		if p.HeaderType == "http" {
			req := obj{"path": []string{first(p.Path, "/")}}
			if p.Host != "" {
				req["headers"] = obj{"Host": strings.Split(p.Host, ",")}
			}
			ss["tcpSettings"] = obj{"header": obj{"type": "http", "request": req}}
		}
	}

	switch p.Security {
	case "tls":
		t := obj{"serverName": first(p.SNI, p.Host, p.Address), "fingerprint": first(p.Fingerprint, "chrome")}
		if p.ALPN != "" {
			t["alpn"] = strings.Split(p.ALPN, ",")
		}
		if p.AllowInsecure {
			t["allowInsecure"] = true
		}
		ss["security"] = "tls"
		ss["tlsSettings"] = t
	case "reality":
		r := obj{
			"serverName":  p.SNI,
			"fingerprint": first(p.Fingerprint, "chrome"),
			"publicKey":   p.PublicKey,
			"shortId":     p.ShortID,
		}
		if p.SpiderX != "" {
			r["spiderX"] = p.SpiderX
		}
		if p.MLDSA65Verify != "" {
			r["mldsa65Verify"] = p.MLDSA65Verify
		}
		ss["security"] = "reality"
		ss["realitySettings"] = r
	default:
		ss["security"] = "none"
	}
	return ss
}

// splitDirect раскладывает свои исключения на домены и IP.
func splitDirect(items []string) (domains, ips []string) {
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		_, _, cidrErr := net.ParseCIDR(it)
		switch {
		case net.ParseIP(it) != nil || cidrErr == nil || strings.HasPrefix(it, "geoip:"):
			ips = append(ips, it)
		case hasAnyPrefix(it, "domain:", "full:", "regexp:", "keyword:", "geosite:"):
			domains = append(domains, it)
		default:
			domains = append(domains, "domain:"+it) // сам домен и все поддомены
		}
	}
	return
}

func routingRules(st *State, hasGeo bool) []obj {
	rules := []obj{
		// SSH (и Git по SSH) — всегда мимо VPN.
		{"type": "field", "port": "22", "outboundTag": "direct"},
		{"type": "field", "ip": privateCIDRs, "outboundTag": "direct"},
		{"type": "field", "domain": []string{"full:localhost", "domain:local"}, "outboundTag": "direct"},
	}
	domains, ips := splitDirect(st.Direct)
	if !hasGeo { // без geo-файлов geoip:/geosite: сломают запуск
		domains = filterPrefix(domains, "geosite:")
		ips = filterPrefix(ips, "geoip:")
	}
	if len(domains) > 0 {
		rules = append(rules, obj{"type": "field", "domain": domains, "outboundTag": "direct"})
	}
	if len(ips) > 0 {
		rules = append(rules, obj{"type": "field", "ip": ips, "outboundTag": "direct"})
	}
	if st.Mode == "ru" {
		d := []string{"domain:ru", "domain:su", "domain:xn--p1ai"}
		if hasGeo {
			d = append(d, "geosite:category-ru")
			rules = append(rules, obj{"type": "field", "ip": []string{"geoip:ru"}, "outboundTag": "direct"})
		}
		rules = append(rules, obj{"type": "field", "domain": d, "outboundTag": "direct"})
	}
	return rules
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func filterPrefix(items []string, prefix string) []string {
	var out []string
	for _, it := range items {
		if !strings.HasPrefix(it, prefix) {
			out = append(out, it)
		}
	}
	return out
}

func sniffing() obj {
	return obj{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true}
}

// buildConfig — конфиг для обычной работы: SOCKS + HTTP на localhost.
func buildConfig(st *State, p Profile, hasGeo bool) ([]byte, error) {
	out, err := outbound(p, "proxy")
	if err != nil {
		return nil, err
	}
	cfg := obj{
		"log": obj{"loglevel": "warning"},
		"inbounds": []obj{
			{"tag": "socks", "listen": "127.0.0.1", "port": st.SocksPort, "protocol": "socks",
				"settings": obj{"udp": true}, "sniffing": sniffing()},
			{"tag": "http", "listen": "127.0.0.1", "port": st.HTTPPort, "protocol": "http",
				"sniffing": sniffing()},
		},
		"outbounds": []obj{
			out,
			{"tag": "direct", "protocol": "freedom"},
			{"tag": "block", "protocol": "blackhole"},
		},
		"routing": obj{"domainStrategy": "AsIs", "rules": routingRules(st, hasGeo)},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// buildPingConfig — один процесс xray, у каждого сервера свой SOCKS-порт.
func buildPingConfig(profiles []Profile, ports []int) ([]byte, []bool) {
	var inb, outb, rules []obj
	ok := make([]bool, len(profiles))
	for i, p := range profiles {
		o, err := outbound(p, fmt.Sprintf("p%d", i))
		if err != nil {
			continue
		}
		ok[i] = true
		inb = append(inb, obj{"tag": fmt.Sprintf("in%d", i), "listen": "127.0.0.1", "port": ports[i], "protocol": "socks"})
		outb = append(outb, o)
		rules = append(rules, obj{"type": "field", "inboundTag": []string{fmt.Sprintf("in%d", i)}, "outboundTag": fmt.Sprintf("p%d", i)})
	}
	outb = append(outb, obj{"tag": "direct", "protocol": "freedom"})
	cfg := obj{
		"log":       obj{"loglevel": "none"},
		"inbounds":  inb,
		"outbounds": outb,
		"routing":   obj{"rules": rules},
	}
	data, _ := json.Marshal(cfg)
	return data, ok
}
