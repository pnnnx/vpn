package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"
)

type SubInfo struct {
	Title    string `json:"title,omitempty"`
	Upload   int64  `json:"upload,omitempty"`
	Download int64  `json:"download,omitempty"`
	Total    int64  `json:"total,omitempty"`
	Expire   int64  `json:"expire,omitempty"`
}

type State struct {
	SubURL     string    `json:"sub_url,omitempty"`
	SubInfo    *SubInfo  `json:"sub_info,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	Profiles   []Profile `json:"profiles"`
	Manual     []Profile `json:"manual,omitempty"` // добавленные вручную через vpn add
	Selected   int       `json:"selected"`         // индекс в All(), -1 — не выбран
	SocksPort  int       `json:"socks_port"`
	HTTPPort   int       `json:"http_port"`
	Mode       string    `json:"mode"`             // global | ru
	Direct     []string  `json:"direct,omitempty"` // свои исключения
	NoSysProxy bool      `json:"no_sys_proxy"`     // не трогать системный прокси

	Enabled      bool `json:"enabled"`        // VPN должен быть включён (для vpn watch)
	NoAutoSwitch bool `json:"no_auto_switch"` // не переключать сервер при падении

	NoSubAutoUpdate bool   `json:"no_sub_auto_update"` // не обновлять подписку раз в сутки
	Lang            string `json:"lang,omitempty"`     // ru | en | "" — как в системе
	TUN             bool   `json:"tun"`                // экспериментальный TUN-режим (нужен root)
}

func (s *State) MetricsPort() int { return s.SocksPort + 2 }

// All — серверы подписки + добавленные вручную.
func (s *State) All() []Profile {
	out := make([]Profile, 0, len(s.Profiles)+len(s.Manual))
	out = append(out, s.Profiles...)
	return append(out, s.Manual...)
}

func (s *State) Current() (Profile, bool) {
	all := s.All()
	if s.Selected < 0 || s.Selected >= len(all) {
		return Profile{}, false
	}
	return all[s.Selected], true
}

func homeDir() string {
	if d := os.Getenv("VPN_HOME"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	// sudo vpn on (TUN): данные остаются в домашней папке пользователя, а не root.
	if os.Geteuid() == 0 && os.Getenv("SUDO_USER") != "" {
		if u, err := user.Lookup(os.Getenv("SUDO_USER")); err == nil {
			h = u.HomeDir
		}
	}
	return filepath.Join(h, ".config", "vpn")
}

func statePath() string { return filepath.Join(homeDir(), "state.json") }

func loadState() (*State, error) {
	st := &State{Selected: -1, SocksPort: 10808, HTTPPort: 10809, Mode: "global"}
	data, err := os.ReadFile(statePath())
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, err
	}
	if st.SocksPort == 0 {
		st.SocksPort = 10808
	}
	if st.HTTPPort == 0 {
		st.HTTPPort = 10809
	}
	if st.Mode == "" {
		st.Mode = "global"
	}
	return st, nil
}

// fixOwner: после sudo возвращаем файлы пользователю, иначе без sudo их будет не изменить.
func fixOwner() {
	if os.Geteuid() != 0 {
		return
	}
	uid, err1 := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, err2 := strconv.Atoi(os.Getenv("SUDO_GID"))
	if err1 != nil || err2 != nil {
		return
	}
	filepath.Walk(homeDir(), func(p string, _ os.FileInfo, err error) error {
		if err == nil {
			os.Lchown(p, uid, gid)
		}
		return nil
	})
}

func (s *State) Save() error {
	if err := os.MkdirAll(homeDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(s, "", "  ")
	tmp := statePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // там ключи — только владельцу
		return err
	}
	err := os.Rename(tmp, statePath())
	fixOwner()
	return err
}
