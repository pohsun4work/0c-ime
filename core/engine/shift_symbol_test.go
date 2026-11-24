package engine

import (
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/proto"
)

// #258：設定「Shift＋符號打半形」由引擎帶給每個 session；沒在組字時這些鍵交還應用程式，
// idle_keys 不能再列（Windows 的 TestKeyDown 照它決定要不要吃）。未按 Shift 的 [ ] 照舊。
func TestShiftSymbolHalfwidthReachesSessionsAndIdleKeys(t *testing.T) {
	e := testEngine()
	if !strings.ContainsAny(e.IdleKeys(), "(){}:") {
		t.Fatalf("預設 Shift 符號由核心打全形：%q", e.IdleKeys())
	}
	e.ShiftSymbolHalfwidth = true
	if !containsAll(e.IdleKeys(), "(){}:?!~&^_<>\"") {
		t.Fatalf("Shift＋符號打半形時閒置的 Shift 符號也由核心處理（fork）：%q", e.IdleKeys())
	}
	if !strings.Contains(e.IdleKeys(), "[") || !strings.Contains(e.IdleKeys(), "`") {
		t.Fatalf("[ 與 ` 照舊由核心處理：%q", e.IdleKeys())
	}
	id := e.OpenSession()
	if snap := styleDo(t, e, id, proto.Request{Type: proto.Key, Key: "("}); !snap.Handled || snap.Text() != "(" {
		t.Fatalf("閒置的 ( 留在組字區（fork）：%+v", snap)
	}
	if snap := styleDo(t, e, id, proto.Request{Type: proto.Key, Key: "<"}); !snap.Handled || snap.Text() != "(," {
		t.Fatalf("Shift＋, 打半形逗號：%+v", snap)
	}
	styleDo(t, e, id, proto.Request{Type: proto.Key, Key: proto.KeyEnter})
	if snap := styleDo(t, e, id, proto.Request{Type: proto.Key, Key: "["}); !snap.Handled || snap.Text() != "「" {
		t.Fatalf("[ 照舊打「：%+v", snap)
	}
	if snap := styleDo(t, e, id, proto.Request{Type: proto.Key, Key: "("}); !snap.Handled || snap.Text() != "「(" {
		t.Fatalf("組字中的 ( 是半形：%+v", snap)
	}
}

// 微軟參考風格的 \ ' " < > ? 打半形原字元，開了設定也照風格（#272）。fork 沒在組字時也留在組字區，閒置按鍵要列。
func TestShiftSymbolHalfwidthMicrosoftIdleKeys(t *testing.T) {
	e := testEngine()
	p, _ := inputstyle.Get("microsoft")
	e.InputStyle = p.Rules
	e.ShiftSymbolHalfwidth = true
	keys := e.IdleKeys()
	if !containsAll(keys, "\\'\"<>?(){}:[") { // fork：沒在組字時也留在組字區
		t.Fatalf("微軟風格＋Shift＋符號打半形的閒置按鍵：%q", keys)
	}
	// 閒置按鍵要跟 session 實際的處理一致（Windows 的 TestKeyDown 照它決定要不要吃）。
	for r := rune(0x21); r <= 0x7e; r++ {
		id := e.OpenSession()
		snap := styleDo(t, e, id, proto.Request{Type: proto.Key, Key: string(r)})
		if want := strings.ContainsRune(keys, r); snap.Handled != want {
			t.Errorf("微軟風格＋設定：%q 實際 handled=%v，IdleKeys 回報 %v", r, snap.Handled, want)
		}
		e.CloseSession(id)
	}
}
