package session

import (
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
)

// 中英混打的括號照微軟一律半形，括號內外的中文、英文照常組字（巢狀、不成對都照打的送出）。
func TestMixedBracketsStayHalfWidth(t *testing.T) {
	for _, keys := range []string{
		"ji3(DNS)", "DNS(ji3)", "ji3[DNS]", "DNS[ji3]", "ji3{DNS}", "DNS{ji3}", "(DNS)",
		"ji3(DNS)ji3", "ji3(DNS) ji3", "ji3(SEE FOO(X))", "FOO(ji3(DNS))", "ji3[DNS(X)]",
		"ji3(DNS[ji3)", "ji3([DNS])", "DNS(ji3)ji3)", "ji3(ji3)", "DNS(DNS)", "DNS)", "ji3)", "ji3(DNS:)",
	} {
		want := strings.ReplaceAll(keys, "ji3", "我")
		s := newSession()
		mixedType(s, keys)
		if got := key(s, proto.KeyEnter).Commit; got != want {
			t.Errorf("%q 提交 %q，要 %q", keys, got, want)
		}
	}
}

// Ctrl＋Shift＋0（#231）是明確要全形，不跟著開括號（#168）；Ctrl＋( 之後一般打的 ) 照微軟是半形。
func TestMixedCtrlParenIsAlwaysFullWidth(t *testing.T) {
	s := newSession()
	mixedType(s, "DNS(ji3")
	key(s, proto.KeyCtrlParenRight)
	if got := key(s, proto.KeyEnter).Commit; got != "DNS(我）" {
		t.Fatalf("Ctrl+)：%q", got)
	}
	s = newSession()
	mixedType(s, "DNS")
	key(s, proto.KeyCtrlParenLeft)
	mixedType(s, "ji3)")
	if got := key(s, proto.KeyEnter).Commit; got != "DNS（我)" {
		t.Fatalf("Ctrl+( 之後的 ) 是半形：%q", got)
	}
}

// 閉括號按 ↓，第二項是全形，選了換得回來（半形 ) 本身不在任何符號組裡，比照 #190 記在原按鍵）。
func TestMixedPairedClosingBracketDownOffersOtherWidth(t *testing.T) {
	for _, tc := range []struct{ keys, before, paired, other string }{
		{"DNS(ji3)", "DNS(我", ")", "）"},
		{"ji3(DNS)", "我(DNS", ")", "）"},
		{"DNS[ji3]", "DNS[我", "]", "」"},
		{"ji3[DNS]", "我[DNS", "]", "」"},
	} {
		s := newSession()
		if snap := mixedType(s, tc.keys); snap.Text() != tc.before+tc.paired {
			t.Fatalf("%q 顯示 %q", tc.keys, snap.Text())
		}
		items := downItems(key(s, proto.KeyDown))
		if len(items) < 2 || items[0] != tc.paired || items[1] != tc.other {
			t.Fatalf("%q 按 ↓：%q", tc.keys, items)
		}
		if snap := s.Handle(proto.Request{Type: proto.Select, Index: 1}); snap.Text() != tc.before+tc.other {
			t.Fatalf("%q 選另一種寬度：%q", tc.keys, snap.Text())
		}
		if got := key(s, proto.KeyEnter).Commit; got != tc.before+tc.other {
			t.Fatalf("%q 提交：%q", tc.keys, got)
		}
	}
}
