package engine

import (
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
)

// #282：所有符號鍵打半形（fork 寫死）；沒在組字時排列的標點鍵也留在組字區（官方交還應用程式），
// idle_keys 要列（Windows 的 TestKeyDown 照它決定要不要吃），答案要跟 session 實際的處理一致。
func TestAllSymbolsHalfwidthIdleKeysMatchSessions(t *testing.T) {
	e := testEngine()
	e.Preview02 = true
	keys := e.IdleKeys()
	if !containsAll(keys, "_(){}[]<>?:\"'\\!~&^`") {
		t.Fatalf("閒置的符號鍵留在組字區（fork）：%q", keys)
	}
	for r := rune(0x21); r <= 0x7e; r++ {
		id := e.OpenSession()
		snap := styleDo(t, e, id, proto.Request{Type: proto.Key, Key: string(r)})
		if want := strings.ContainsRune(keys, r); snap.Handled != want {
			t.Errorf("%q 實際 handled=%v，IdleKeys 回報 %v", r, snap.Handled, want)
		}
		e.CloseSession(id)
	}
	id := e.OpenSession()
	styleDo(t, e, id, proto.Request{Type: proto.Key, Key: proto.KeyCtrlComma})
	if snap := styleDo(t, e, id, proto.Request{Type: proto.Key, Key: "["}); snap.Text() != "，[" {
		t.Fatalf("Ctrl＋, 打全形、[ 打半形：%+v", snap)
	}
}
