//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const agentLabel = "io.github.pnnnx.vpn"

func agentPath() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "LaunchAgents", agentLabel+".plist")
}

func autostartEnabled() bool {
	_, err := os.Stat(agentPath())
	return err == nil
}

func launchctl(args ...string) error {
	if b, err := exec.Command("launchctl", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl %s: %s", args[0], string(b))
	}
	return nil
}

func autostartOn() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	env := ""
	if v := os.Getenv("VPN_HOME"); v != "" {
		env = "<key>EnvironmentVariables</key><dict><key>VPN_HOME</key><string>" + v + "</string></dict>"
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + agentLabel + `</string>
  <key>ProgramArguments</key><array><string>` + exe + `</string><string>watch</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>AbandonProcessGroup</key><true/>
  <key>ProcessType</key><string>Background</string>
  <key>StandardOutPath</key><string>` + watchLogPath() + `</string>
  <key>StandardErrorPath</key><string>` + watchLogPath() + `</string>
  ` + env + `
</dict>
</plist>
`
	os.MkdirAll(filepath.Dir(agentPath()), 0o755)
	os.MkdirAll(homeDir(), 0o700)
	if err := os.WriteFile(agentPath(), []byte(plist), 0o644); err != nil {
		return err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	launchctl("bootout", domain+"/"+agentLabel) // если уже был — перезапускаем
	return launchctl("bootstrap", domain, agentPath())
}

func autostartOff() error {
	launchctl("bootout", "gui/"+strconv.Itoa(os.Getuid())+"/"+agentLabel)
	if err := os.Remove(agentPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
