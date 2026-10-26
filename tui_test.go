package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIRenders(t *testing.T) {
	st := &State{SocksPort: 10808, HTTPPort: 10809, Mode: "global", Selected: 0}
	for _, l := range allLinks() {
		p, _ := ParseLink(l)
		st.Profiles = append(st.Profiles, p)
	}
	m := &model{st: st, w: 60, h: 12}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	for _, k := range []string{"j", "j", "G", "k"} {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
	v := m.View()
	if !strings.Contains(v, "ВЫКЛ") || !strings.Contains(v, "Нидерланды 1") || m.cursor != len(st.Profiles)-2 {
		t.Fatalf("cursor=%d\n%s", m.cursor, v)
	}
	m.Update(pingMsg{res: tcpPingFake(len(st.Profiles))})
	_ = m.View()
	empty := &model{st: &State{Selected: -1}, scr: scInput, inputFor: "sub", w: 30, h: 5}
	empty.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://x")})
	if !strings.Contains(empty.View(), "https://x") {
		t.Fatal("ввод подписки не работает")
	}
}

func tcpPingFake(n int) []PingResult {
	r := make([]PingResult, n)
	for i := range r {
		r[i] = PingResult{Delay: -1}
	}
	return r
}

func TestTUIScreens(t *testing.T) {
	t.Setenv("VPN_HOME", t.TempDir())
	st := &State{SocksPort: 10808, HTTPPort: 10809, Mode: "global", Selected: 0}
	for _, l := range allLinks() {
		p, _ := ParseLink(l)
		st.Profiles = append(st.Profiles, p)
	}
	m := &model{st: st, w: 70, h: 16}
	key := func(s string) { m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }

	key("i")
	if v := m.View(); m.scr != scInfo || strings.Contains(v, fakeUUID) || !strings.Contains(v, "1111••••5555") {
		t.Fatalf("инфо должно скрывать UUID:\n%s", v)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	key("d")
	key("a")
	key("myserver.com 1.2.3.4")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.scr != scDirect || len(m.st.Direct) != 2 {
		t.Fatalf("исключения: scr=%d direct=%v", m.scr, m.st.Direct)
	}
	key("x")
	if len(m.st.Direct) != 1 {
		t.Fatalf("удаление: %v", m.st.Direct)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	key("l")
	if m.scr != scLog || !strings.Contains(m.View(), "xray") {
		t.Fatal("экран лога")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// скорость: два замера счётчиков с интервалом 1 с
	now := time.Now()
	m.applyStatus(statusMsg{running: true, statsOK: true, up: 0, down: 0, at: now})
	m.applyStatus(statusMsg{running: true, statsOK: true, up: 2048, down: 3 << 20, at: now.Add(time.Second)})
	if v := m.View(); !strings.Contains(v, "↓ 3.0 МБ/с") || !strings.Contains(v, "↑ 2 КБ/с") {
		t.Fatalf("скорость:\n%s", v)
	}
}
