package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func xrayAsset() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		return "Xray-macos-arm64-v8a.zip", nil
	case "darwin/amd64":
		return "Xray-macos-64.zip", nil
	case "linux/amd64":
		return "Xray-linux-64.zip", nil
	case "linux/arm64":
		return "Xray-linux-arm64-v8a.zip", nil
	}
	return "", fmt.Errorf("нет сборки Xray для %s/%s", runtime.GOOS, runtime.GOARCH)
}

// setupXray скачивает последний релиз Xray-core и проверяет SHA-256.
func setupXray(st *State, out func(string)) error {
	asset, err := xrayAsset()
	if err != nil {
		return err
	}
	base := "https://github.com/XTLS/Xray-core/releases/latest/download/" + asset
	out("Скачиваю " + asset + "…")
	data, _, err := fetchWithFallback(st, base)
	if err != nil {
		return fmt.Errorf("не удалось скачать Xray: %w", err)
	}
	if dgst, _, err := fetchWithFallback(st, base+".dgst"); err == nil {
		want := ""
		for _, line := range strings.Split(string(dgst), "\n") {
			if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "SHA2-256" {
				want = strings.ToLower(strings.TrimSpace(v))
			}
		}
		sum := sha256.Sum256(data)
		if want != "" && hex.EncodeToString(sum[:]) != want {
			return fmt.Errorf("контрольная сумма не совпала — файл повреждён или подменён")
		}
		out("Контрольная сумма SHA-256 совпала.")
	} else {
		out("Предупреждение: не удалось проверить контрольную сумму.")
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(binDir(), 0o755); err != nil {
		return err
	}
	want := map[string]os.FileMode{"xray": 0o755, "geoip.dat": 0o644, "geosite.dat": 0o644}
	for _, f := range zr.File {
		mode, ok := want[filepath.Base(f.Name)]
		if !ok || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		dst := filepath.Join(binDir(), filepath.Base(f.Name))
		tmp := dst + ".tmp"
		w, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(w, rc)
		rc.Close()
		w.Close()
		if err != nil {
			return err
		}
		if err := os.Rename(tmp, dst); err != nil {
			return err
		}
	}
	out("Xray установлен в " + binDir())
	return nil
}
