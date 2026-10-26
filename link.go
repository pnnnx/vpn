package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Profile — один сервер из подписки или ссылки.
type Profile struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"` // vless, vmess, trojan, shadowsocks
	Address  string `json:"address"`
	Port     int    `json:"port"`

	// Учётные данные
	ID         string `json:"id,omitempty"`         // vless/vmess UUID
	Password   string `json:"password,omitempty"`   // trojan/ss
	Method     string `json:"method,omitempty"`     // ss cipher
	Flow       string `json:"flow,omitempty"`       // vless flow (xtls-rprx-vision)
	Encryption string `json:"encryption,omitempty"` // vless encryption / vmess security
	AlterID    int    `json:"alter_id,omitempty"`

	// Транспорт
	Network     string          `json:"network,omitempty"` // tcp/raw, ws, grpc, xhttp, httpupgrade, kcp
	Path        string          `json:"path,omitempty"`
	Host        string          `json:"host,omitempty"`
	ServiceName string          `json:"service_name,omitempty"`
	Mode        string          `json:"mode,omitempty"`
	HeaderType  string          `json:"header_type,omitempty"`
	Seed        string          `json:"seed,omitempty"`
	Extra       json.RawMessage `json:"extra,omitempty"` // xhttp extra

	// Безопасность
	Security      string `json:"security,omitempty"` // none, tls, reality
	SNI           string `json:"sni,omitempty"`
	ALPN          string `json:"alpn,omitempty"`
	Fingerprint   string `json:"fp,omitempty"`
	PublicKey     string `json:"pbk,omitempty"`
	ShortID       string `json:"sid,omitempty"`
	SpiderX       string `json:"spx,omitempty"`
	MLDSA65Verify string `json:"pqv,omitempty"`
	AllowInsecure bool   `json:"allow_insecure,omitempty"`

	Raw string `json:"raw"`
}

func (p Profile) Endpoint() string {
	return net.JoinHostPort(p.Address, strconv.Itoa(p.Port))
}

func (p Profile) Summary() string {
	parts := []string{p.Protocol}
	if p.Network != "" && p.Network != "tcp" && p.Network != "raw" {
		parts = append(parts, p.Network)
	}
	if p.Security != "" && p.Security != "none" {
		parts = append(parts, p.Security)
	}
	return strings.Join(parts, "+")
}

// ParseLink разбирает vless://, vmess://, trojan://, ss://.
func ParseLink(raw string) (Profile, error) {
	raw = strings.TrimSpace(raw)
	i := strings.Index(raw, "://")
	if i < 0 {
		return Profile{}, fmt.Errorf("не похоже на ссылку: %q", short(raw))
	}
	scheme := strings.ToLower(raw[:i])
	var p Profile
	var err error
	switch scheme {
	case "vless":
		p, err = parseURLStyle(raw, "vless")
	case "trojan":
		p, err = parseURLStyle(raw, "trojan")
	case "vmess":
		p, err = parseVmess(raw)
	case "ss":
		p, err = parseSS(raw)
	default:
		return Profile{}, fmt.Errorf("протокол %s пока не поддерживается", scheme)
	}
	if err != nil {
		return Profile{}, err
	}
	p.Raw = raw
	if p.Name == "" {
		p.Name = p.Endpoint()
	}
	if p.Port <= 0 || p.Port > 65535 || p.Address == "" {
		return Profile{}, fmt.Errorf("в ссылке нет адреса или порта")
	}
	return p, nil
}

func parseURLStyle(raw, proto string) (Profile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Profile{}, fmt.Errorf("%s: %w", proto, err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return Profile{}, fmt.Errorf("%s: неверный порт", proto)
	}
	q := u.Query()
	p := Profile{
		Protocol:      proto,
		Name:          u.Fragment,
		Address:       u.Hostname(),
		Port:          port,
		Network:       strings.ToLower(first(q.Get("type"), "tcp")),
		Path:          q.Get("path"),
		Host:          q.Get("host"),
		ServiceName:   q.Get("serviceName"),
		Mode:          q.Get("mode"),
		HeaderType:    q.Get("headerType"),
		Seed:          q.Get("seed"),
		Security:      strings.ToLower(q.Get("security")),
		SNI:           first(q.Get("sni"), q.Get("peer")),
		ALPN:          q.Get("alpn"),
		Fingerprint:   q.Get("fp"),
		PublicKey:     q.Get("pbk"),
		ShortID:       q.Get("sid"),
		SpiderX:       q.Get("spx"),
		MLDSA65Verify: q.Get("pqv"),
		AllowInsecure: q.Get("allowInsecure") == "1" || q.Get("allowInsecure") == "true" ||
			q.Get("insecure") == "1",
	}
	if ex := q.Get("extra"); ex != "" && json.Valid([]byte(ex)) {
		p.Extra = json.RawMessage(ex)
	}
	user := u.User.Username()
	switch proto {
	case "vless":
		p.ID = user
		p.Flow = q.Get("flow")
		p.Encryption = first(q.Get("encryption"), "none")
		if p.Security == "" {
			p.Security = "none"
		}
	case "trojan":
		p.Password = user
		if pw, ok := u.User.Password(); ok { // trojan://user:pass@ — редкость
			p.Password = user + ":" + pw
		}
		if p.Security == "" {
			p.Security = "tls" // у trojan TLS по умолчанию
		}
	}
	if p.Network == "splithttp" {
		p.Network = "xhttp"
	}
	return p, nil
}

func parseVmess(raw string) (Profile, error) {
	body := strings.TrimPrefix(raw[len("vmess://"):], "")
	if i := strings.Index(body, "#"); i >= 0 {
		body = body[:i]
	}
	data, err := b64(body)
	if err != nil {
		return Profile{}, fmt.Errorf("vmess: не base64")
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return Profile{}, fmt.Errorf("vmess: неверный JSON")
	}
	s := func(k string) string {
		switch v := m[k].(type) {
		case string:
			return v
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
		return ""
	}
	port, _ := strconv.Atoi(s("port"))
	aid, _ := strconv.Atoi(s("aid"))
	p := Profile{
		Protocol:    "vmess",
		Name:        s("ps"),
		Address:     s("add"),
		Port:        port,
		ID:          s("id"),
		AlterID:     aid,
		Encryption:  first(s("scy"), "auto"),
		Network:     strings.ToLower(first(s("net"), "tcp")),
		HeaderType:  s("type"),
		Host:        s("host"),
		Path:        s("path"),
		Security:    strings.ToLower(s("tls")),
		SNI:         s("sni"),
		ALPN:        s("alpn"),
		Fingerprint: s("fp"),
	}
	if p.Network == "grpc" {
		p.ServiceName = p.Path
		if p.HeaderType == "multi" {
			p.Mode = "multi"
		}
	}
	if p.Security == "" {
		p.Security = "none"
	}
	return p, nil
}

func parseSS(raw string) (Profile, error) {
	rest := raw[len("ss://"):]
	name := ""
	if i := strings.Index(rest, "#"); i >= 0 {
		name, _ = url.PathUnescape(rest[i+1:])
		rest = rest[:i]
	}
	if i := strings.Index(rest, "?"); i >= 0 { // plugin=... не поддерживаем
		rest = rest[:i]
	}
	rest = strings.TrimSuffix(rest, "/")
	var userinfo, hostport string
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		userinfo, hostport = rest[:at], rest[at+1:]
		if dec, err := b64(userinfo); err == nil && strings.Contains(string(dec), ":") {
			userinfo = string(dec)
		} else if un, err := url.PathUnescape(userinfo); err == nil {
			userinfo = un
		}
	} else { // старый формат: всё в base64
		dec, err := b64(rest)
		if err != nil {
			return Profile{}, fmt.Errorf("ss: не удалось разобрать")
		}
		s := string(dec)
		at := strings.LastIndex(s, "@")
		if at < 0 {
			return Profile{}, fmt.Errorf("ss: нет адреса")
		}
		userinfo, hostport = s[:at], s[at+1:]
	}
	method, password, ok := strings.Cut(userinfo, ":")
	if !ok {
		return Profile{}, fmt.Errorf("ss: нет метода/пароля")
	}
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return Profile{}, fmt.Errorf("ss: неверный адрес")
	}
	port, _ := strconv.Atoi(portStr)
	return Profile{
		Protocol: "shadowsocks",
		Name:     name,
		Address:  host,
		Port:     port,
		Method:   method,
		Password: password,
		Security: "none",
	}, nil
}

func b64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if d, err := enc.DecodeString(s); err == nil {
			return d, nil
		}
	}
	return nil, fmt.Errorf("not base64")
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func short(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}
