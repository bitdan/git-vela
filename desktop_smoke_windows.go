//go:build desktop_smoke && !production

package main

import "path/filepath"

func init() {
	windowsOptions.WebviewUserDataPath, _ = filepath.Abs(".tools/webview-smoke")
	windowsOptions.AdditionalBrowserArgs = []string{"--remote-debugging-port=9331"}
}
