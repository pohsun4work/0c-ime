package settings

import "testing"

// 引擎 health 回報的啟動配置換回命令列寫法後，跟每一項的值相同：0c-update 才不會把已套用的設定顯示成待套用。
// 補標點看模型狀態；沒有回報的項目（舊引擎）不出現。
func TestHealthInfoRoundTripsEveryItem(t *testing.T) {
	s := Default()
	for _, kv := range [][2]string{{"selection", Statistical}, {"punctuation", "on"}, {"document-context", "off"}, {"toggle-key", ToggleLeft},
		{"shift-tap-ms", "500"}, {"start-mode", StartEnglish}, {"selection-keys", SelectQwer}, {"tab", "cycle"}, {"page-size", "10"}, {"fuzzy-s-sh", "off"}, {"fuzzy-z-zh", "auto"}} {
		if err := s.Set(kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	info := s.HealthInfo()
	if info["toggle_key"] != ToggleLeft || info["document_context"] != "false" || info["selection_setting"] != Statistical || info["shift_space"] != "disabled" {
		t.Fatalf("外殼與舊的 0c-update 讀的鍵：%v", info)
	}
	if _, ok := info["punctuation"]; ok {
		t.Fatal("補標點由引擎另外回報")
	}
	info["punctuation_state"] = "ready"
	running := RunningValues(info)
	for _, it := range s.Items() {
		if running[it.Key] != it.Value {
			t.Errorf("%s：health 換回 %q，設定是 %q", it.Key, running[it.Key], it.Value)
		}
	}
	if len(running) != len(s.Items()) {
		t.Fatalf("多出項目：%v", running)
	}
	if got := RunningValues(map[string]string{"punctuation_state": "disabled_by_setting"}); len(got) != 1 || got["punctuation"] != "off" {
		t.Fatalf("%v", got)
	}
	if got := RunningValues(map[string]string{"version": "old", "esc_clear": "maybe"}); len(got) != 0 {
		t.Fatalf("沒有回報或讀不懂的值不出現：%v", got)
	}
}
