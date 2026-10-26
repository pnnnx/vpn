package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("VPN_LANG", "ru") // тесты проверяют русские строки
	os.Exit(m.Run())
}

func TestL(t *testing.T) {
	if L("да", "yes") != "да" {
		t.Fatal("ожидался русский")
	}
	langEN = true
	defer func() { langEN = false }()
	if L("да", "yes") != "yes" {
		t.Fatal("ожидался английский")
	}
}

func TestTUNConfigAndAutoUpdateDue(t *testing.T) {
	p, _ := ParseLink(linkReality)
	st := &State{SocksPort: 10808, HTTPPort: 10809, Mode: "global", TUN: true}
	cfg, err := buildConfig(st, p, false)
	if err != nil || !contains(string(cfg), `"protocol": "tun"`) || !contains(string(cfg), `"autoOutboundsInterface": "auto"`) {
		t.Fatalf("нет TUN-inbound:\n%s", cfg)
	}
	if subUpdateDue(&State{}) {
		t.Error("без подписки обновлять нечего")
	}
	if !subUpdateDue(&State{SubURL: "https://x"}) {
		t.Error("давно не обновляли — пора")
	}
	if subUpdateDue(&State{SubURL: "https://x", NoSubAutoUpdate: true}) {
		t.Error("автообновление выключено")
	}
}
