package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Все ключи и UUID ниже — фейковые.
const (
	fakeUUID    = "11111111-2222-3333-4444-555555555555"
	linkReality = "vless://" + fakeUUID + "@203.0.113.10:443?type=tcp&security=reality&pbk=ZXhhbXBsZXB1YmxpY2tleWZvcnRlc3RzMTIzNDU2Nzg&sid=abcd1234&fp=chrome&sni=www.example.com&flow=xtls-rprx-vision&encryption=none#%D0%9D%D0%B8%D0%B4%D0%B5%D1%80%D0%BB%D0%B0%D0%BD%D0%B4%D1%8B%201"
	linkWS      = "vless://" + fakeUUID + "@example.org:443?type=ws&security=tls&path=%2Fws&host=cdn.example.org&sni=cdn.example.org#ws"
	linkXHTTP   = "vless://" + fakeUUID + "@[2001:db8::1]:8443?type=xhttp&security=reality&pbk=ZXhhbXBsZXB1YmxpY2tleWZvcnRlc3RzMTIzNDU2Nzg&sid=ab&sni=www.example.com&path=%2Fx&mode=stream-one#xhttp"
	linkGRPC    = "vless://" + fakeUUID + "@example.org:443?type=grpc&security=tls&serviceName=grpcsvc&sni=example.org#grpc"
	linkTrojan  = "trojan://secretpass@example.net:443?sni=example.net#trojan"
	linkSS      = "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNzd29yZA@198.51.100.5:8388#ss"
)

func linkVmess() string {
	j := `{"v":"2","ps":"vmess-ws","add":"example.com","port":"443","id":"` + fakeUUID + `","aid":"0","scy":"auto","net":"ws","type":"none","host":"example.com","path":"/v","tls":"tls","sni":"example.com"}`
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(j))
}

func allLinks() []string {
	return []string{linkReality, linkWS, linkXHTTP, linkGRPC, linkTrojan, linkSS, linkVmess()}
}

func TestParseReality(t *testing.T) {
	p, err := ParseLink(linkReality)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Нидерланды 1" || p.Address != "203.0.113.10" || p.Port != 443 ||
		p.Security != "reality" || p.Flow != "xtls-rprx-vision" || p.ShortID != "abcd1234" || p.SNI != "www.example.com" {
		t.Fatalf("неверно: %+v", p)
	}
}

func TestParseIPv6AndXHTTP(t *testing.T) {
	p, err := ParseLink(linkXHTTP)
	if err != nil {
		t.Fatal(err)
	}
	if p.Address != "2001:db8::1" || p.Port != 8443 || p.Network != "xhttp" || p.Mode != "stream-one" {
		t.Fatalf("неверно: %+v", p)
	}
	if p.Endpoint() != "[2001:db8::1]:8443" {
		t.Fatal(p.Endpoint())
	}
}

func TestParseVmess(t *testing.T) {
	p, err := ParseLink(linkVmess())
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "vmess-ws" || p.Port != 443 || p.Network != "ws" || p.Security != "tls" || p.Path != "/v" {
		t.Fatalf("неверно: %+v", p)
	}
}

func TestParseTrojanDefaultsTLS(t *testing.T) {
	p, err := ParseLink(linkTrojan)
	if err != nil {
		t.Fatal(err)
	}
	if p.Password != "secretpass" || p.Security != "tls" {
		t.Fatalf("неверно: %+v", p)
	}
}

func TestParseSS(t *testing.T) {
	for _, l := range []string{
		linkSS,
		"ss://chacha20-ietf-poly1305:password@198.51.100.5:8388#plain",
		"ss://" + base64.StdEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:password@198.51.100.5:8388")) + "#legacy",
	} {
		p, err := ParseLink(l)
		if err != nil {
			t.Fatalf("%s: %v", l, err)
		}
		if p.Method != "chacha20-ietf-poly1305" || p.Password != "password" || p.Port != 8388 {
			t.Fatalf("%s: %+v", l, p)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, l := range []string{"hello", "vless://x@host", "wireguard://abc", "vmess://!!!"} {
		if _, err := ParseLink(l); err == nil {
			t.Errorf("ожидалась ошибка для %q", l)
		}
	}
}

func TestDecodeSub(t *testing.T) {
	plain := linkReality + "\n" + linkTrojan + "\n\n"
	for _, body := range []string{plain, base64.StdEncoding.EncodeToString([]byte(plain)), base64.RawURLEncoding.EncodeToString([]byte(plain))} {
		if got := decodeSub([]byte(body)); len(got) != 2 {
			t.Fatalf("ожидалось 2 ссылки, получено %d", len(got))
		}
	}
}

func TestSubInfo(t *testing.T) {
	h := http.Header{}
	h.Set("Subscription-Userinfo", "upload=100; download=200; total=1000; expire=1893456000")
	h.Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte("Мой VPN")))
	si := parseSubInfo(h)
	if si == nil || si.Upload != 100 || si.Download != 200 || si.Total != 1000 || si.Expire != 1893456000 || si.Title != "Мой VPN" {
		t.Fatalf("%+v", si)
	}
}

func TestRoutingSSHDirect(t *testing.T) {
	st := &State{SocksPort: 10808, HTTPPort: 10809, Mode: "ru", Direct: []string{"1.2.3.4", "10.8.0.0/24", "myserver.com", "full:x.org", "geoip:cn"}}
	rules := routingRules(st, false)
	first := rules[0]
	if first["port"] != "22" || first["outboundTag"] != "direct" {
		t.Fatalf("первое правило должно отправлять SSH напрямую: %v", first)
	}
	b, _ := json.Marshal(rules)
	s := string(b)
	for _, want := range []string{`"1.2.3.4"`, `"10.8.0.0/24"`, `"domain:myserver.com"`, `"full:x.org"`, `"domain:ru"`} {
		if !contains(s, want) {
			t.Errorf("нет %s в правилах", want)
		}
	}
	if contains(s, "geoip:") || contains(s, "geosite:") {
		t.Error("без geo-файлов geoip:/geosite: добавлять нельзя")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool { return indexOf(s, sub) >= 0 })()
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestXrayAcceptsConfigs — интеграционный: запускается, если задан XRAY_BIN.
func TestXrayAcceptsConfigs(t *testing.T) {
	xray := os.Getenv("XRAY_BIN")
	if xray == "" {
		t.Skip("XRAY_BIN не задан")
	}
	assets := filepath.Dir(xray)
	for _, mode := range []string{"global", "ru"} {
		for _, l := range allLinks() {
			p, err := ParseLink(l)
			if err != nil {
				t.Fatal(err)
			}
			st := &State{SocksPort: 10808, HTTPPort: 10809, Mode: mode, Direct: []string{"myserver.com", "1.2.3.4"}}
			cfg, err := buildConfig(st, p, true)
			if err != nil {
				t.Fatal(err)
			}
			f := filepath.Join(t.TempDir(), "c.json")
			os.WriteFile(f, cfg, 0o600)
			cmd := exec.Command(xray, "run", "-test", "-c", f)
			cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+assets)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s [%s]: xray отклонил конфиг:\n%s", p.Name, mode, out)
			}
		}
	}
}
