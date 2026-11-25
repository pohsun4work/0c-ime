package session

import (
	"testing"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// repairSession 收 P6 規格例子用到的字；英文字典有 a、hello、the、sq、go、an、kevin，沒有 ck、sql。
func repairSession() *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range []struct{ w, r string }{
		{"停", "ㄊㄧㄥˊ"}, {"聽", "ㄊㄧㄥ"}, {"營", "ㄧㄥˊ"}, {"什", "ㄕㄜˊ"}, {"我", "ㄨㄛˇ"}, {"你", "ㄋㄧˇ"},
		{"孬", "ㄋㄠ"}, {"架", "ㄐㄧㄚˋ"}, {"碼", "ㄇㄚˇ"}, {"天", "ㄊㄧㄢ"}, {"女", "ㄋㄩˇ"}, {"設", "ㄕㄜˋ"},
		{"很", "ㄏㄣˇ"}, {"篇", "ㄆㄧㄢ"}, {"昨", "ㄗㄨㄛˊ"}, {"蘇", "ㄙㄨ"}, {"以", "ㄧˇ"}, {"時", "ㄕˊ"},
		{"跑", "ㄆㄠˇ"}, {"傻", "ㄕㄚˇ"}, {"廣", "ㄍㄨㄤˇ"}, {"塞", "ㄙㄟ"}, {"正", "ㄓㄥˋ"}, {"婚", "ㄏㄨㄣ"},
		{"溫", "ㄨㄣ"}, {"驗", "ㄧㄢˋ"}, {"電", "ㄉㄧㄢˋ"}, {"深", "ㄕㄣ"}, {"年", "ㄋㄧㄢˊ"}, {"爸", "ㄅㄚˋ"},
	} {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, 1e-3)
	}
	s := New(personal.NewModel(lex, sys))
	var words []english.Word
	for _, w := range []string{"a", "hello", "the", "sq", "go", "an", "kevin"} {
		words = append(words, english.Word{Text: w, Weight: 65})
	}
	s.English = english.New(words)
	return s
}

func repairCommit(s *Session, keys string) string {
	mixedType(s, keys)
	return key(s, proto.KeyEnter).Commit
}

// P6 冗餘音（照華碩）：同一個注音重複按當成一次，相鄰、不相鄰、字母、數字、標點都算；結尾是空白也合併。
func TestMixedRedundantKeys(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"wuuuu/6", "停"}, {"wu//6", "停"}, {"wuwu/6", "停"}, {"ggggk6", "什"}, {"gkgk6", "什"},
		{"ji3wuuuu/6su3", "我停你"},
		{"hellowu//6", "hello停"}, {"thewuuu/6", "the停"}, // 英文字後面照樣合併
		{"wu// ", "聽"}, {"ssl ", "孬"}, // 字典沒收的英文會變字
		// 數字鍵加聲調鍵：沒有介音 3 個以內、有介音 4 個以內才合併
		{"ru8884", "架"}, {"a883", "碼"}, {"a8883", "a8883"}, {"2u0004", "2u0004"},
		{"2025su006", "2025年"}, // 前面的數字不算這個音節的
	} {
		if got := repairCommit(repairSession(), tc.keys); got != tc.want {
			t.Errorf("%q 送出 %q，要 %q", tc.keys, got, tc.want)
		}
	}
}

// 設定「重複按的注音當成一次」關閉時照官方：多按的鍵照字面留下。
func TestMixedRedundantKeysOff(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"wuuuu/6", "wuuu營"}, {"wu// ", "wu// "},
		{"hkg6", "什"}, // 打錯鍵照常修正
	} {
		s := repairSession()
		s.KeepRedundantKeys = true
		if got := repairCommit(s, tc.keys); got != tc.want {
			t.Errorf("%q 送出 %q，要 %q", tc.keys, got, tc.want)
		}
	}
}

// P6 打錯鍵（照華碩）：聲母按到 3×3 相鄰的鍵丟掉、不相鄰的切成英文；介音、韻母同一格後按的算；
// 數字鍵聲母緊接聲母時留數字。
func TestMixedTypoKeys(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"hkg6", "什"}, {"ckg6", "c什"}, {"qwu0 ", "天"}, {"gjn ", "蘇"}, {"5jiy6", "昨"},
		{"sju3", "你"}, {"sujm3", "女"}, {"g8k4", "設"}, {"cip3", "很"}, {"ju3", "以"},
		{"1qu0 ", "1篇"},
		{"xwu/6", "x停"}, {"cgk6", "c什"}, // 不相鄰的聲母只有一個字母時，後段照順序打也切
		{"asu3", "a你"}, // 英文字搶後面的字母照官方
	} {
		if got := repairCommit(repairSession(), tc.keys); got != tc.want {
			t.Errorf("%q 送出 %q，要 %q", tc.keys, got, tc.want)
		}
	}
}

// 冗餘音、打錯鍵不拿英文的字母來修正。
func TestMixedRepairKeepsEnglish(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"go83", "go83"},            // go 的 o 不是打錯的韻母（不是 傻）
		{"sqlql3", "sql跑"},          // sq 的 s、q 不跟後面的 ql 合（不是 跑、sq跑）
		{"anccp3", "anc很"},          // an 後面的 c 不跟 cp 合（不是 an很）
		{"youtubeej;3", "youtube廣"}, // 英文字尾的 e 不跟 ej; 合
		{"kevinji3", "kevin我"},
		{"Dyson ", "Dyson "},                       // 接空白不在不相鄰的聲母切開（不是 Dys塞）
		{"15/4", "15/4"},                           // 數字寫法不是數字接注音（不是 1正）
		{"ji31984", "我1984"}, {"ji31.84", "我1.84"}, // 只有數字、標點的不修正（不是 我爸）
		{"cpujp ", "cpujp "}, // 冗餘音和打錯鍵不疊在同一個音節（不是 婚）
	} {
		if got := repairCommit(repairSession(), tc.keys); got != tc.want {
			t.Errorf("%q 送出 %q，要 %q", tc.keys, got, tc.want)
		}
	}
}

func TestKeysAdjacent(t *testing.T) {
	for _, tc := range []struct {
		a, b rune
		want bool
	}{
		{'h', 'g', true}, {'f', 'g', true}, {'n', 'g', true}, {'t', 'g', true},
		{'c', 'g', false}, {'d', 'g', false}, {'x', 'w', false}, {'5', 'y', true}, {'1', 'q', true}, {'5', '1', false},
	} {
		if got := keysAdjacent(tc.a, tc.b); got != tc.want {
			t.Errorf("keysAdjacent(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}
