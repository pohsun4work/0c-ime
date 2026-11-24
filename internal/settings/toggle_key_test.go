package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

// #206：預設左右 Shift 都切換；存過的選擇改其他設定時保留。
func TestToggleKeyDefaultAndSavedChoices(t *testing.T) {
	for _, body := range []string{`{}`, `{"mixed_input":true}`} {
		var s Settings
		if err := json.Unmarshal([]byte(body), &s); err != nil {
			t.Fatal(err)
		}
		if s.ToggleKey != ToggleBoth || !s.TogglesRight() || !s.TogglesLeft() {
			t.Fatalf("缺少切換鍵設定時使用預設：%s → %+v", body, s)
		}
	}
	for _, value := range []string{ToggleRight, ToggleBoth, ToggleLeft, ToggleNone} {
		var s Settings
		if err := json.Unmarshal([]byte(`{"toggle_key":"`+value+`"}`), &s); err != nil {
			t.Fatal(err)
		}
		if err := s.Set("selection", Statistical); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var back Settings
		if err := json.Unmarshal(data, &back); err != nil || back.ToggleKey != value {
			t.Fatalf("更改其他設定後保留已儲存的切換鍵：%s → %+v %v", data, back, err)
		}
	}
	if d := Default(); d.Set("toggle-key", "ctrl") == nil {
		t.Fatal("不認得的值")
	}
	for _, item := range Default().Items() {
		if item.Key == "toggle-key" && (item.Default != ToggleBoth || item.Value != ToggleBoth || item.ValueLabels[ToggleBoth] != "左右 Shift") {
			t.Fatalf("設定程式的預設與目前值應一致：%+v", item)
		}
	}
	// #117：Ctrl＋空白鍵是 Windows 繁中輸入法的系統熱鍵。
	if !strings.HasSuffix(toggleKeyDescription, "也可以按 Ctrl＋空白鍵") {
		t.Fatal(toggleKeyDescription)
	}
}
