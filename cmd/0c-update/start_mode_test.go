package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/internal/settings"
)

// settings 的文字清單要列出設定程式看得到的每一項（曾經漏過 toggle-key、mode-memory）。
func TestPrintSettingsListsEveryItem(t *testing.T) {
	s := settings.Default()
	s.Set("start-mode", "english")
	var out bytes.Buffer
	listSettings(&out, settings.Report{Path: "/tmp/settings.json", Settings: s})
	text := out.String()
	for _, item := range s.Items() {
		if !strings.Contains(text, "\n"+item.Key+"=") {
			t.Errorf("沒有列出 %s：\n%s", item.Key, text)
		}
	}
	if !strings.Contains(text, "\nstart-mode=english（") {
		t.Fatal(text)
	}
}
