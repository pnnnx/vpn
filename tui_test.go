package main

import (
	"strings"
	"testing"

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
	empty := &model{st: &State{Selected: -1}, input: true, w: 30, h: 5}
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
