package session

import (
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/proto"
)

// P5：組不成字的按鍵照字面。顯示成注音、還沒打完的待定按鍵：
// 空白、Enter 時單鍵或加聲調組得成字的照官方留下注音（c␣ 是 ㄏ，#190；ji Enter 是 ㄨㄛ），
// 多鍵又怎樣都組不成字的照原按鍵（npm␣ 是 npm␣）；接符號、數字鍵盤一律照原按鍵，按過的鍵不丟。
func TestUnreachableKeysStayAsTyped(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"npm ", "npm "},
		{"cpu ", "cpu "},
		{"c ", "ㄏ"},
		{"ji ", "ㄨㄛ"},
		{"nm\n", "nm"},
		{"v\n", "ㄒ"},
		{"ji\n", "ㄨㄛ"},
		{"e(", "e("},
		{"e\tKP2\te", "e2e"},
		{"ji3npm ji3", "我npm 我"},
	} {
		s := newSession()
		keys := splitProbeKeys(tc.keys)
		if keys[len(keys)-1] != proto.KeyEnter {
			keys = append(keys, proto.KeyEnter)
		}
		var snap proto.Snapshot
		for _, k := range keys {
			snap = key(s, k)
		}
		if got := snap.Commit; got != tc.want {
			t.Errorf("%q 送出 %q，要 %q", tc.keys, got, tc.want)
		}
	}
}

// 照原按鍵留下的待定按鍵，退格回到它後面時收回接著組（ji 數字鍵盤 1 退格再打 3 是 我）；英文模式不收回。
func TestPendingKeysRestore(t *testing.T) {
	s := newSession()
	for _, k := range []string{"j", "i", "KP1"} {
		key(s, k)
	}
	if snap := key(s, proto.KeyBackspace); snap.Text() != "ㄨㄛ" {
		t.Fatalf("退格刪掉數字後收回待定按鍵：%+v", snap)
	}
	if snap := key(s, "3"); snap.Text() != "我" {
		t.Fatalf("收回後接著組：%+v", snap)
	}
	s = newSession()
	for _, k := range []string{"j", "i", "KP1"} {
		key(s, k)
	}
	s.Handle(proto.Request{Type: proto.Toggle})
	key(s, proto.KeyBackspace)
	if snap := key(s, "a"); snap.Text() != "jia" {
		t.Fatalf("英文模式照字面接在後面：%+v", snap)
	}
}

// splitProbeKeys：\t 分隔具名按鍵（KP2），\n 是 Enter，其他逐字。
func splitProbeKeys(keys string) []string {
	var out []string
	for i := 0; i < len(keys); i++ {
		switch keys[i] {
		case '\n':
			out = append(out, proto.KeyEnter)
		case '\t':
			j := i + 1
			for j < len(keys) && keys[j] != '\t' {
				j++
			}
			out = append(out, keys[i+1:j])
			i = j
		default:
			out = append(out, keys[i:i+1])
		}
	}
	return out
}

// ↓ 選了注音解釋的（choice 2），Enter、數字鍵盤、移動游標定案時照注音，不改回原按鍵。
func TestPickedZhuyinSettlesAsZhuyin(t *testing.T) {
	for _, tc := range []struct {
		then []string
		want string
	}{
		{[]string{proto.KeyEnter}, "ㄙㄩ"},
		{[]string{"KP1", proto.KeyEnter}, "ㄙㄩ1"},
		{[]string{proto.KeyLeft, proto.KeyEnter}, "ㄙㄩ"},
	} {
		s := newSession()
		mixedType(s, "nm")
		key(s, proto.KeyDown)
		pickText(t, s, "ㄙㄩ")
		var snap proto.Snapshot
		for _, k := range tc.then {
			snap = key(s, k)
		}
		if snap.Commit != tc.want {
			t.Errorf("nm ↓ㄙㄩ %q 送出 %q，要 %q", tc.then, snap.Commit, tc.want)
		}
	}
}

// 點注音鍵（proto.Symbol）接在顯示成注音的待定按鍵後面：交給注音接著組（nj 再點 ㄅ 是 ㄅㄨ）；
// 照原按鍵留下的不被注音鍵收回（nj ← → 再點 ㄅ 是 njㄅ）。
func TestSymbolAfterPendingKeys(t *testing.T) {
	tap := func(s *Session, syms string) proto.Snapshot {
		var snap proto.Snapshot
		for _, r := range syms {
			snap = s.Handle(proto.Request{Type: proto.Symbol, Symbol: string(r)})
		}
		return snap
	}
	for _, tc := range []struct{ keys, syms, want string }{
		{"nj", "ㄅ", "ㄅㄨ"},
		{"ji", "ˇ", "我"},
		{"e", "ㄅㄚ", "ㄅㄚ"},
	} {
		s := newSession()
		mixedType(s, tc.keys)
		if got := tap(s, tc.syms).Text(); got != tc.want {
			t.Errorf("%s 再點 %s：%q，要 %q", tc.keys, tc.syms, got, tc.want)
		}
	}
	s := newSession()
	mixedType(s, "nj")
	key(s, proto.KeyLeft)
	key(s, proto.KeyRight)
	if got := tap(s, "ㄅ").Text(); got != "njㄅ" {
		t.Errorf("nj ← → 再點 ㄅ：%q", got)
	}
}

// 切換字寬前照原按鍵留下的不收回，照切換前的字寬定案；同一字寬照常收回。
func TestPendingKeysKeepWidth(t *testing.T) {
	s := newSession()
	s.InputStyle.ShiftSpace = inputstyle.ShiftSpaceFullWidth
	key(s, proto.KeyShiftSpace)
	mixedType(s, "nm")
	key(s, proto.KeyShiftSpace)
	if got := key(s, "x").Text(); !strings.HasPrefix(got, "ｎｍ") {
		t.Errorf("全形 nm 切回半形再打 x：%q", got)
	}
	s = newSession()
	s.InputStyle.ShiftSpace = inputstyle.ShiftSpaceFullWidth
	key(s, proto.KeyShiftSpace)
	for _, k := range []string{"j", "i", "KP1", proto.KeyBackspace} {
		key(s, k)
	}
	if got := key(s, "3").Text(); got != "我" {
		t.Errorf("全形 ji 數字鍵盤 1 退格再打 3：%q", got)
	}
}
