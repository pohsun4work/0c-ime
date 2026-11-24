package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

// #160：開始時的中英模式。舊檔沒有這個欄位時是注音（跟以前一樣）；讀不懂的值退回注音、存檔時原樣寫回。
// 不用 Shift 切換時照樣從設定的模式開始：Windows 可以按 Ctrl＋空白鍵或點工作列的中／英切回注音。
func TestStartModeDefaultAndCompat(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{"mode_memory":"native"}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.StartMode != StartZhuyin || s.StartsEnglish() {
		t.Fatalf("舊檔沒有欄位：%+v", s)
	}
	if Default().StartMode != StartZhuyin {
		t.Fatal("預設注音")
	}
	json.Unmarshal([]byte(`{"toggle_key":"none","start_mode":"english"}`), &s)
	if !s.StartsEnglish() {
		t.Fatal("english")
	}
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"start_mode":"english"`) {
		t.Fatalf("寫回：%s", b)
	}
	json.Unmarshal([]byte(`{"start_mode":"latin"}`), &s)
	if s.StartsEnglish() {
		t.Fatal("讀不懂的值退回注音")
	}
	if b, _ = json.Marshal(s); !strings.Contains(string(b), `"start_mode":"latin"`) {
		t.Fatalf("讀不懂的值原樣寫回：%s", b)
	}
	if notes := strings.Join(s.notes(), "\n"); !strings.Contains(notes, "start-mode") {
		t.Fatalf("說明退回：%s", notes)
	}
	if err := s.Set("start-mode", "english"); err != nil || !s.StartsEnglish() {
		t.Fatal(err)
	}
	if b, _ = json.Marshal(s); strings.Contains(string(b), `"latin"`) {
		t.Fatalf("明確設定後不再寫回舊值：%s", b)
	}
	if err := s.Set("start-mode", "chinese"); err == nil {
		t.Fatal("不認得的值")
	}
	if v, ok := s.Value("start-mode"); !ok || v != StartEnglish {
		t.Fatal(v)
	}
	for _, item := range s.Items() {
		if item.Key == "start-mode" && (item.Disabled || item.ValueLabels[StartZhuyin] != "注音" || item.ValueLabels[StartEnglish] != "英文") {
			t.Fatalf("%+v", item)
		}
		if item.Key == "mode-memory" && item.ValueLabels[ModeNative] != "每個程式各自記住" {
			t.Fatalf("%+v", item)
		}
	}
}
