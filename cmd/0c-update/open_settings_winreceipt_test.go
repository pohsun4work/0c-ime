package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeReceipt(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "install-receipt.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstalledSettingsFromReceiptReadsTheVersionDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "Zuin")
	receipt := writeReceipt(t, `{"version":"0.1.0-dev.43","directory":"versions/0.1.0-dev.43-abc123","active":true,
		"files":{"0c_tsf.dll":"d","0c-settings.exe":"bbb"}}`)
	path, hash := installedFromReceipt(receipt, base, "0c-settings.exe")
	if path != filepath.Join(base, "versions", "0.1.0-dev.43-abc123", "0c-settings.exe") || hash != "bbb" {
		t.Fatalf("%s %s", path, hash)
	}
	for name, body := range map[string]string{
		"舊套件沒有設定程式": `{"directory":"versions/x","active":true,"files":{"0c_tsf.dll":"d"}}`,
		"已解除安裝":     `{"directory":"versions/x","active":false,"files":{"0c-settings.exe":"b"}}`,
		"絕對路徑":      `{"directory":"C:/Windows","active":true,"files":{"0c-settings.exe":"b"}}`,
		"跳出目錄":      `{"directory":"../elsewhere","active":true,"files":{"0c-settings.exe":"b"}}`,
		"損毀":        `{not json`,
	} {
		if path, hash := installedFromReceipt(writeReceipt(t, body), base, "0c-settings.exe"); path != "" || hash != "" {
			t.Errorf("%s：%q %q", name, path, hash)
		}
	}
	if path, _ := installedFromReceipt(filepath.Join(base, "absent.json"), base, "0c-settings.exe"); path != "" {
		t.Error("沒有收據")
	}
}
