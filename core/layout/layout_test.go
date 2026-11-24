package layout

import (
	"testing"

	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

func TestStandardRoundTrip(t *testing.T) {
	for syl, keys := range map[zhuyin.Syllable]string{
		"ㄑㄧㄣ": "fup ", "ㄗˇ": "y3", "ㄐㄧㄥˇ": "ru/3", "ㄉㄨㄥˋ": "2j/4", "ㄇㄚ˙": "a87", "ㄔ": "t ",
	} {
		if got := Standard.Keys(syl); got != keys {
			t.Errorf("Keys(%s)=%q 應為 %q", syl, got, keys)
		}
		var c zhuyin.Composer
		var done zhuyin.Syllable
		for _, k := range keys {
			sym, ok := Standard.Symbol(k)
			if !ok {
				t.Fatalf("按鍵 %q 無對應", k)
			}
			if s, fin := c.Add(sym); fin {
				done = s
			}
		}
		if done != syl {
			t.Errorf("按鍵 %q 組出 %q 應為 %q", keys, done, syl)
		}
	}
}

func TestStandardPunct(t *testing.T) {
	for key, want := range map[rune]string{'<': "，", '>': "。", '?': "？", '\'': "、", '\\': "、", '&': "＆", '^': "……", '_': "——"} {
		if got, ok := Standard.Punct(key); !ok || got != want {
			t.Errorf("Punct(%q)=%q,%v 應為 %q", key, got, ok, want)
		}
	}
}

func TestPunctDoesNotShadowPhonetic(t *testing.T) {
	// 標點不可與注音鍵重疊，否則那個標點永遠打不出來。
	for _, key := range `<>?:"!~[]{}\()&'^_` {
		if _, ok := Standard.Punct(key); ok {
			if sym, used := Standard.Symbol(key); used {
				t.Errorf("%q 同時是注音 %q 與標點", key, sym)
			}
		}
	}
}
