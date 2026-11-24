package session

import (
	"reflect"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
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
	// 游標在中間時插在游標處；顯示成注音、還沒打完的待定按鍵照原按鍵留下（v[ 不是 ㄒ[；_ ^ 是識別字的一部分，#118）。
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
	if snap := key(s, "["); snap.Text() != "我v[" {
		t.Fatalf("未打完的注音照原按鍵留下：%+v", snap)
	}
	s = newSession()
	typeSyl(s, "ㄨㄛˇ")
	key(s, "v")
	if snap := key(s, "_"); snap.Text() != "我v_" {
		t.Fatalf("_ 是識別字的一部分：%+v", snap)
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
// 收下的符號後面接聲調組成中文時從符號後面切開（DNS<我、OK?我），接空白照字面（DNS<b␣；a<b␣ 見 TestMixedEnglishKeepsStylePunctuation）。
func TestAllSymbolsHalfwidthMixedInput(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"ji3_", "我_"},
		{"ji3(DNS)", "我(DNS)"},
		{"DNS(ji3)", "DNS(我)"},
		{"ji3[DNS]", "我[DNS]"},
		{"ji3<", "我<"},
		{"DNS<ji3", "DNS<我"},
		{"OK?ji3", "OK?我"},
		{"DNS:ji3", "DNS:我"},
		{"ji3OK<ji3", "我OK<我"},
		{"DNS<b ", "DNS<b "},
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

// 併進英文的符號定案時（Enter、↓）單獨留下：神經選字時 Enter 也不能掉字，前段照畫面送出（up? 不能送出 ㄧㄣ?），
// ↓ 開的是游標前那個符號（第二項是全形），↑ 不動還沒定案的按鍵。
func TestJoinedSymbolSettles(t *testing.T) {
	calls := 0
	for _, keys := range []string{"ji3DNS<", "DNS?", "ji3DNS:"} {
		s := neuralSession(&calls)
		mixedType(s, keys)
		want := strings.ReplaceAll(keys, "ji3", "我")
		if got := key(s, proto.KeyEnter).Commit; got != want {
			t.Errorf("神經選字 %q Enter：%q，要 %q", keys, got, want)
		}
	}
	upSession := func() *Session {
		lex, sys := lexicon.NewMem(), lm.NewMem()
		lex.Add(lexicon.Entry{Surface: "因", Reading: zhuyin.ParseReading("ㄧㄣ"), PRW: 1})
		sys.SetUni("因", 1e-3)
		s := New(personal.NewModel(lex, sys))
		s.English = english.New([]english.Word{{Text: "up", Weight: 10}})
		return s
	}
	if got := mixedType(upSession(), "up").Text(); got != "ㄧㄣ" {
		t.Fatalf("前提：up 單獨顯示成注音，實際 %q", got)
	}
	s := upSession()
	shown := mixedType(s, "up?").Text()
	if got := key(s, proto.KeyEnter).Commit; got != shown || got != "up?" {
		t.Errorf("up? 畫面 %q、送出 %q", shown, got)
	}
	// 句中（游標後面還有字）按 ↓ 也是 < 的候選。
	s = newSession()
	mixedType(s, "ji3t ")
	key(s, proto.KeyLeft)
	mixedType(s, "DNS<")
	if items := downItems(key(s, proto.KeyDown)); len(items) < 2 || items[0] != "<" || items[1] != "，" {
		t.Errorf("句中 DNS< 按 ↓：%q", items)
	}
	// 從符號後面切開（DNS<我）時留下的 < 按 ↓ 也換得到全形。
	s = newSession()
	mixedType(s, "DNS<ji3")
	s.Handle(proto.Request{Type: proto.Cursor, To: 3})
	if items := downItems(key(s, proto.KeyDown)); len(items) < 2 || items[0] != "<" || items[1] != "，" {
		t.Errorf("DNS<我 的 < 按 ↓：%q", items)
	}
	s = newSession()
	mixedType(s, "DNS:")
	key(s, proto.KeyUp)
	mixedType(s, "a")
	if got := key(s, proto.KeyEnter).Commit; got != "DNS:a" {
		t.Errorf("DNS: ↑ 之後接著打 a：%q", got)
	}
}

// 符號後面只有數字的是數字寫法（時間、編號），不切開組成中文；全形時的 03:00 也照數字寫法。
func TestJoinedSymbolBeforeDigitsStaysNumber(t *testing.T) {
	for _, keys := range []string{"10:04", "12:53", "10:56", "DNS:04", "ji3 10:54"} {
		s := numberWritingSession()
		want := strings.ReplaceAll(keys, "ji3", "我")
		if got := mixedType(s, keys).Text(); got != want {
			t.Errorf("%q：%q，要 %q", keys, got, want)
		}
	}
	s := numberWritingSession()
	s.InputStyle.ShiftSpace = inputstyle.ShiftSpaceFullWidth
	key(s, proto.KeyShiftSpace)
	if got := mixedType(s, "03:00").Text(); got != "０３：００" {
		t.Errorf("全形 03:00：%q", got)
	}
}
