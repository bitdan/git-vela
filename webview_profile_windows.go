//go:build !desktop_smoke || production

package main

import (
	"os"
	"path/filepath"
)

func init() {
	// WebView2 names its default profile after the executable. Keep the
	// existing profile when upgrading from gitvela.exe to git-vela.exe.
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return
	}
	legacy := filepath.Join(appData, "gitvela.exe")
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		windowsOptions.WebviewUserDataPath = legacy
	}
}
