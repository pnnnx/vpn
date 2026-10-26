package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Многие панели (Marzban, Remnawave, 3x-ui) отдают список ссылок только «знакомым» клиентам.
const subUserAgent = "v2rayN/7.10.0"

// fetchTimeout — 20 с для подписки; setup увеличивает его для скачивания ядра.
var fetchTimeout = 20 * time.Second

func fetchURL(rawURL, proxy string) ([]byte, http.Header, error) {
	tr := &http.Transport{Proxy: nil}
	if proxy != "" {
		pu, _ := url.Parse(proxy)
		tr.Proxy = http.ProxyURL(pu)
	}
	cl := &http.Client{Transport: tr, Timeout: fetchTimeout}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", subUserAgent)
	resp, err := cl.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("сервер ответил %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	return body, resp.Header, err
}

// fetchWithFallback: сначала напрямую, а если не вышло и VPN запущен — через него.
func fetchWithFallback(st *State, rawURL string) ([]byte, http.Header, error) {
	body, hdr, err := fetchURL(rawURL, "")
	if err == nil {
		return body, hdr, nil
	}
	if running, _ := isRunning(); running {
		proxy := fmt.Sprintf("http://127.0.0.1:%d", st.HTTPPort)
		if b, h, err2 := fetchURL(rawURL, proxy); err2 == nil {
			return b, h, nil
		}
	}
	return nil, nil, err
}

func decodeSub(body []byte) []string {
	s := strings.TrimSpace(string(body))
	if !strings.Contains(s, "://") {
		if dec, err := b64(s); err == nil {
			s = string(dec)
		}
	}
	var links []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "://") {
			links = append(links, line)
		}
	}
	return links
}

func parseSubInfo(h http.Header) *SubInfo {
	info := &SubInfo{}
	for _, part := range strings.Split(h.Get("Subscription-Userinfo"), ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		switch strings.ToLower(k) {
		case "upload":
			info.Upload = n
		case "download":
			info.Download = n
		case "total":
			info.Total = n
		case "expire":
			info.Expire = n
		}
	}
	t := h.Get("Profile-Title")
	if strings.HasPrefix(t, "base64:") {
		if d, err := b64(t[7:]); err == nil {
			t = string(d)
		}
	}
	info.Title = t
	if *info == (SubInfo{}) {
		return nil
	}
	return info
}

func updateSub(st *State) (int, []string, error) {
	if st.SubURL == "" {
		return 0, nil, fmt.Errorf("подписка не задана: vpn sub <url>")
	}
	body, hdr, err := fetchWithFallback(st, st.SubURL)
	if err != nil {
		return 0, nil, fmt.Errorf("не удалось скачать подписку: %w", err)
	}
	links := decodeSub(body)
	if len(links) == 0 {
		return 0, nil, fmt.Errorf("в ответе нет ссылок (первые байты: %q)", short(string(body)))
	}
	var prev Profile
	hadPrev := false
	if cur, ok := st.Current(); ok {
		prev, hadPrev = cur, true
	}
	var profiles []Profile
	var skipped []string
	for _, l := range links {
		p, err := ParseLink(l)
		if err != nil {
			skipped = append(skipped, err.Error())
			continue
		}
		profiles = append(profiles, p)
	}
	st.Profiles = profiles
	st.SubInfo = parseSubInfo(hdr)
	st.UpdatedAt = time.Now()
	// Сохраняем выбор, если тот же сервер остался в подписке.
	st.Selected = -1
	if hadPrev {
		for i, p := range st.All() {
			if p.Name == prev.Name && p.Endpoint() == prev.Endpoint() {
				st.Selected = i
				break
			}
		}
		if st.Selected < 0 {
			for i, p := range st.All() {
				if p.Endpoint() == prev.Endpoint() {
					st.Selected = i
					break
				}
			}
		}
	}
	if st.Selected < 0 && len(st.All()) > 0 {
		st.Selected = 0
	}
	return len(profiles), skipped, nil
}
