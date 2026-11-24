package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain 讓整個測試行程（以及它啟動的 0c-server 子行程）用臨時的使用者目錄：引擎啟動時會清 onnxruntime 的遙測暫存
// （neural.RemoveTelemetryQueue），預設的資料與個人檔也在 LOCALAPPDATA 底下，不可以動到跑測試的人自己的資料。
// 個別測試仍可另外指定。測試執行檔也兼當 stopServer 送 Ctrl+Break 的小工具（ctrlBreakEnv）與假的新執行檔（fakeEngineEnv）。
func TestMain(m *testing.M) {
	if pid := os.Getenv(ctrlBreakEnv); pid != "" {
		os.Exit(ctrlBreakMain(pid))
	}
	if mode := os.Getenv(fakeEngineEnv); mode != "" {
		os.Exit(fakeEngineMain(mode))
	}
	buildEnv = os.Environ()
	root, err := os.MkdirTemp("", "0c-server-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binDir = filepath.Join(root, "bin")
	for _, dir := range []string{binDir, filepath.Join(root, "home"), filepath.Join(root, "local"), filepath.Join(root, "roaming")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Setenv("USERPROFILE", filepath.Join(root, "home"))
	os.Setenv("HOME", filepath.Join(root, "home"))
	os.Setenv("LOCALAPPDATA", filepath.Join(root, "local"))
	os.Setenv("APPDATA", filepath.Join(root, "roaming"))
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
