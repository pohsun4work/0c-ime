package session

import (
	"reflect"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/proto"
)

// #282：所有符號鍵打半形（fork 寫死）。注音模式的符號鍵不論有沒有按 Shift 都打鍵帽上的半形字元
// （Shift＋- 是 _、Shift＋, 是 <、[ 是 [），留在組字區、Enter 一起送出；全形標點用 Ctrl＋符號。

// standardPunctKeys 是標準排列有全形標點的按鍵（standardPunct）。
var standardPunctKeys = []string{"<", ">", "?", ":", "\"", "!", "~", "[", "]", "{", "}", "'", "\\", "(", ")", "&", "^", "_"}

// fork：沒在組字時也留在組字區（明確打的符號，hoshi_symbols.go），官方是交還應用程式。
func TestAllSymbolsHalfwidthIdleKeysStayInComposition(t *testing.T) {
	for _, k := range standardPunctKeys {
		s := newSession()
		if snap := key(s, k); !snap.Handled || snap.Commit != "" || snap.Text() != k {
			t.Fatalf("沒在組字時 %q 應以半形留在組字區：%+v", k, snap)
		}
	}
}

func TestAllSymbolsHalfwidthInsideComposition(t *testing.T) {
	for _, k := range standardPunctKeys {
		s := newSession()
		typeSyl(s, "ㄨㄛˇ")
		if snap := key(s, k); !snap.Handled || snap.Commit != "" || snap.Text() != "我"+k {
			t.Fatalf("組字中 %q 應以半形留在組字區：%+v", k, snap)
		}
		if snap := typeSyl(s, "ㄒㄧㄤˇ"); snap.Text() != "我"+k+"想" {
			t.Fatalf("%q 之後應能繼續打字：%+v", k, snap)
		}
		if snap := key(s, proto.KeyEnter); snap.Commit != "我"+k+"想" {
			t.Fatalf("Enter 送出 %q：%+v", k, snap)
		}
	}
	// 游標在中間時插在游標處；未打完的注音同其他標點留成符號。
	s := newSession()
	typeSyl(s, "ㄨㄛˇ")
	typeSyl(s, "ㄒㄧㄤˇ")
	key(s, proto.KeyLeft)
	if snap := key(s, "_"); snap.Text() != "我_想" || snap.Cursor.Rune != 2 {
		t.Fatalf("應插在游標處：%+v", snap)
	}
	s = newSession()
	typeSyl(s, "ㄨㄛˇ")
	key(s, "v") // ㄒ
	if snap := key(s, "_"); snap.Text() != "我ㄒ_" {
		t.Fatalf("未打完的注音應留成符號：%+v", snap)
	}
}

// ↓ 第二項是這一鍵原本的全形標點，選得回去；兩項都不重複。
func TestAllSymbolsHalfwidthDownOffersFullWidth(t *testing.T) {
	for _, k := range standardPunctKeys {
		full, _ := layout.Standard.Punct([]rune(k)[0])
		s := newSession()
		typeSyl(s, "ㄨㄛˇ")
		key(s, k)
		key(s, proto.KeyDown)
		got := candidateTexts(s)
		if len(got) < 2 || got[0] != k || got[1] != full {
			t.Fatalf("%q 的 ↓ 應以 %q %q 開頭：%q", k, k, full, got)
		}
		for _, w := range []string{k, full} {
			if strings.Count("\x00"+strings.Join(got, "\x00")+"\x00", "\x00"+w+"\x00") != 1 {
				t.Fatalf("%q 的 ↓ 不應重複列 %q：%q", k, w, got)
			}
		}
		key(s, "2")
		if snap := key(s, proto.KeyEnter); snap.Commit != "我"+full {
			t.Fatalf("選第二項應換成全形 %q：%+v", full, snap)
		}
	}
}

// Ctrl＋符號打全形。
func TestAllSymbolsHalfwidthCtrlTypesFullWidth(t *testing.T) {
	for _, tc := range ctrlPunctKeys {
		s := newSession()
		if snap := key(s, tc.key); !snap.Handled || snap.Text() != tc.want {
			t.Fatalf("%s 應打全形 %q：%+v", tc.key, tc.want, snap)
		}
	}
}

// 英文模式交還應用程式；候選開著時吃掉不動作；Shift＋, 是鍵帽的 <。
func TestAllSymbolsHalfwidthOtherStates(t *testing.T) {
	s := newSession()
	s.Handle(proto.Request{Type: proto.Mode, On: true})
	if snap := key(s, "_"); snap.Handled {
		t.Fatalf("英文模式交還應用程式：%+v", snap)
	}
	s = newSession()
	typeSyl(s, "ㄨㄛˇ")
	key(s, proto.KeyDown)
	before := candidateTexts(s)
	if snap := key(s, "_"); !snap.Handled || snap.Text() != "我" || !reflect.DeepEqual(candidateTexts(s), before) {
		t.Fatalf("候選開著時吃掉不動作：%+v", snap)
	}
	s = newSession()
	if snap := key(s, "<"); !snap.Handled || snap.Text() != "<" {
		t.Fatalf("沒在組字時 Shift＋, 是 <、留在組字區（fork）：%+v", snap)
	}
}

// 中英混打：打到一半的英文收下識別字、網址、路徑裡的符號，括號讓英文先定案；中文後面照樣半形。
func TestAllSymbolsHalfwidthMixedInput(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"ji3_", "我_"},
		{"ji3(DNS)", "我(DNS)"},
		{"DNS(ji3)", "DNS(我)"},
		{"ji3[DNS]", "我[DNS]"},
		{"ji3<", "我<"},
		{"DNS<ji3", "DNS<我"},
		{"user_name", "user_name"},
		{"search?q=1", "search?q=1"},
		{"C:\\Users", "C:\\Users"},
		{"don't", "don't"},
		{"su_", "su_"},
		{"ji3 3:00", "我 3:00"},
	} {
		s := newSession()
		mixedType(s, tc.keys)
		if got := key(s, proto.KeyEnter).Commit; got != tc.want {
			t.Errorf("%q 提交 %q，要 %q", tc.keys, got, tc.want)
		}
	}
}

// 閒置按鍵（Windows 的 TestKeyDown）：IdleKeys 要跟 session 實際的處理一致。
func TestAllSymbolsHalfwidthIdleKeysMatchSession(t *testing.T) {
	idle := IdleKeys()
	for r := rune(0x21); r <= 0x7e; r++ {
		s := newSession()
		s.Preview02 = true
		got := key(s, string(r)).Handled
		if want := strings.ContainsRune(idle, r); got != want {
			t.Errorf("%q 實際 handled=%v，IdleKeys 回報 %v", r, got, want)
		}
	}
}
