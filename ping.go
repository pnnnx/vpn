package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"time"
)

const testURL = "https://www.gstatic.com/generate_204"

// PingResult: Delay < 0 — недоступен.
type PingResult struct {
	Delay time.Duration
	Err   error
}

func tcpPing(profiles []Profile) []PingResult {
	res := make([]PingResult, len(profiles))
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for i, p := range profiles {
		wg.Add(1)
		go func(i int, p Profile) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			t := time.Now()
			c, err := net.DialTimeout("tcp", p.Endpoint(), 3*time.Second)
			if err != nil {
				res[i] = PingResult{Delay: -1, Err: err}
				return
			}
			c.Close()
			res[i] = PingResult{Delay: time.Since(t)}
		}(i, p)
	}
	wg.Wait()
	return res
}

func freePorts(n int) ([]int, error) {
	var ls []net.Listener
	var ports []int
	defer func() {
		for _, l := range ls {
			l.Close()
		}
	}()
	for i := 0; i < n; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		ls = append(ls, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	return ports, nil
}

// realPing поднимает временный xray: у каждого сервера свой SOCKS-порт.
func realPing(st *State, profiles []Profile, out func(string)) ([]PingResult, error) {
	xray, err := ensureXray(st, out)
	if err != nil {
		return nil, err
	}
	ports, err := freePorts(len(profiles))
	if err != nil {
		return nil, err
	}
	cfg, ok := buildPingConfig(profiles, ports)
	f, err := os.CreateTemp("", "vpn-ping-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	f.Write(cfg)
	f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, xray, "run", "-c", f.Name())
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	defer func() { cancel(); cmd.Wait() }() // сначала убить, потом дождаться
	for i := 0; i < 40; i++ {               // ждём, пока поднимется хотя бы один порт
		if portOpen(ports[0]) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	res := make([]PingResult, len(profiles))
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for i := range profiles {
		if !ok[i] {
			res[i] = PingResult{Delay: -1, Err: fmt.Errorf("не поддерживается")}
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			d, err := httpDelay(fmt.Sprintf("socks5://127.0.0.1:%d", ports[i]), 6*time.Second)
			if err != nil {
				res[i] = PingResult{Delay: -1, Err: err}
			} else {
				res[i] = PingResult{Delay: d}
			}
		}(i)
	}
	wg.Wait()
	return res, nil
}

func httpDelay(proxy string, timeout time.Duration) (time.Duration, error) {
	pu, _ := url.Parse(proxy)
	cl := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(pu), DisableKeepAlives: true},
		Timeout:   timeout,
	}
	t := time.Now()
	resp, err := cl.Get(testURL)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return 0, fmt.Errorf("ответ %d", resp.StatusCode)
	}
	return time.Since(t), nil
}

func fmtDelay(r PingResult) string {
	if r.Delay < 0 {
		return "—"
	}
	if r.Delay == 0 {
		return ""
	}
	return fmt.Sprintf("%d мс", r.Delay.Milliseconds())
}
