package settings

import "testing"

// 設定程式把各項分在不同頁面（#207）：停用的項目要說出是哪一項造成的，才能帶人到那一頁去改。
// 新增停用原因卻忘了在 disabledBy 登記時，這裡會失敗。
func TestDisabledItemsNameTheirCause(t *testing.T) {
	keys := map[string]bool{}
	for _, f := range fields() {
		keys[f.key] = true
	}
	for _, toggle := range []string{ToggleRight, ToggleBoth, ToggleLeft, ToggleNone} {
		s := Default()
		s.ToggleKey, s.StartMode = toggle, StartEnglish
		for _, it := range s.Items() {
			if it.Disabled != (it.DisabledBy != "") {
				t.Errorf("%s：%s 的 disabled=%v 但 disabled_by=%q", toggle, it.Key, it.Disabled, it.DisabledBy)
			}
			if it.Disabled && (!keys[it.DisabledBy] || it.DisabledBy == it.Key) {
				t.Errorf("%s：%s 停用（%s）沒有指出原因所在的設定（%q）", toggle, it.Key, it.DisabledReason, it.DisabledBy)
			}
		}
	}
	none := Default()
	none.ToggleKey = ToggleNone
	if by := disabledBy(none.disabledReason("shift-tap-ms")); by != "toggle-key" {
		t.Fatalf("不用 Shift 時的單按時限要指向中英切換鍵：%q", by)
	}
	if Default().disabledReason("shift-tap-ms") != "" {
		t.Fatal("用 Shift 切換時單按時限照常")
	}
}
