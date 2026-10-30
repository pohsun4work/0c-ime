package session

import (
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// overwriteSession 是整合風格的混打 session；table 不是 nil 時接上假的神經模型（memoryScorer，沒列的字 -10）。
func overwriteSession(table map[string]float64) *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range []word{
		{"我", "ㄨㄛˇ", 1e-2}, {"昨", "ㄗㄨㄛˊ", 1e-3}, {"天", "ㄊㄧㄢ", 3e-3}, {"昨天", "ㄗㄨㄛˊ ㄊㄧㄢ", 1e-3},
		{"速", "ㄙㄨˋ", 1e-3}, {"次", "ㄘˋ", 2e-3}, {"著", "ㄓㄨㄛˊ", 1e-3}, {"蘇", "ㄙㄨ", 1e-4}, {"個", "ㄍㄜ˙", 3e-3},
		{"合", "ㄏㄜˊ", 1e-3}, {"哪", "ㄋㄟ˙", 1e-4}, {"偶", "ㄡˇ", 1e-4}, {"吼", "ㄏㄡˇ", 1e-4}, {"雖", "ㄙㄨㄟ", 1e-3},
		{"片", "ㄆㄧㄢˋ", 1e-3}, {"什", "ㄕㄜˊ", 2e-3},
	} {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, e.p)
	}
	s := New(personal.NewModel(lex, sys))
	p, _ := inputstyle.Get("blend")
	s.InputStyle = p.Rules
	s.MixedInput, s.CtrlPunct = true, true
	var words []english.Word
	for _, w := range []string{"a", "is", "hello", "shining", "ok", "web", "pi"} {
		words = append(words, english.Word{Text: w, Weight: 3})
	}
	s.English = english.New(words)
	if table != nil {
		var calls int
		s.Rerank = rerank.Options{Weight: .1, TextWeight: 1, ContextRunes: rerank.MaxContextRunes, Scorer: memoryScorer(table, &calls)}
	}
	return s
}

func TestOverwriteReading(t *testing.T) {
	s := overwriteSession(nil)
	for _, tc := range []struct {
		keys, want string
		lead       bool
	}{
		{"5jiy6", "ㄗㄨㄛˊ", false}, // ㄓㄨㄛ 後補 ㄗ
		{"5jyi6", "ㄗㄨㄛˊ", false},
		{"gjn4", "ㄙㄨˋ", false},
		{"ruf4", "ㄑㄧˋ", false},
		{"5j8i6", "ㄓㄨㄛˊ", false}, // ㄚ 後補 ㄛ
		{"rum4", "ㄐㄩˋ", false},   // ㄧ 後補 ㄩ
		{"cl.3", "ㄏㄡˇ", false},
		{"gjn ", "ㄙㄨ", false}, // 空白是一聲
		{"gnj4", "ㄙㄨˋ", true}, // 前面多一個聲母
		{"nh4", "ㄘˋ", true},
		{"5yji6", "ㄗㄨㄛˊ", true},
	} {
		o, ok := s.overwriteReading(tc.keys)
		syl, _ := o.group.syllable()
		if !ok || string(syl) != tc.want || o.lead != tc.lead || o.group.keys != tc.keys {
			t.Errorf("%s：%q lead=%v ok=%v，要 %s lead=%v", tc.keys, syl, o.lead, ok, tc.want, tc.lead)
		}
	}
	for _, keys := range []string{
		"5ji6",     // 沒有重複
		"ggggk6",   // 同一個鍵連按（mixed_context 的重複鍵）
		"iphone6",  // 好幾格重複
		"win7",     // 蓋完 ㄙㄛ 不是國語音節
		"5jiy",     // 沒有聲調
		"5jiy6abc", // 聲調鍵不在最後
	} {
		if o, ok := s.overwriteReading(keys); ok {
			t.Errorf("%s 不算：%+v", keys, o)
		}
	}
}

// 神經模型照前文判斷要不要照微軟「同一格後按的蓋掉先按的」換；分數是 2026-10-30 本機實測的值。
func TestMixedOverwriteNeural(t *testing.T) {
	for _, tc := range []struct {
		left  string
		keys  []string
		table map[string]float64
		want  string
	}{
		// 中間隔開、緊貼的韻母：蓋完的字比照字面的亂碼合理得多。
		{"", []string{"ji3", "5jiy6"}, map[string]float64{"我昨": -2.55}, "我昨"},
		{"他睡", []string{"5j8i6"}, map[string]float64{"他睡著": -0.92}, "著"},
		// 官方切開的「cl偶」也一樣看。
		{"不要", []string{"cl.3"}, map[string]float64{"不要吼": -4.52}, "吼"},
		// 英文型號照字面很順：留著。
		{"升級到", []string{"ios7"}, map[string]float64{"升級到哪": -3.20, "升級到i": -3, "升級到io": -2, "升級到ios": -2, "升級到ios7": -1.88}, "ios7"},
		// 前面多一個聲母：字母讓後面的字不合理才蓋掉（g速），更合理就留著（n次、5個）。
		{"車子的", []string{"gnj4"}, map[string]float64{"車子的速": -2.89, "車子的g速": -6.21}, "速"},
		{"我講過", []string{"nh4"}, map[string]float64{"我講過次": -3.49, "我講過n次": -2.77}, "n次"},
		{"買了", []string{"5ek7"}, map[string]float64{"買了個": -2.80, "買了5個": -2.13}, "5個"},
		// 一聲：門檻比聲調鍵低；開發常用的字照字面很順，留著。
		{"他姓", []string{"gjn "}, map[string]float64{"他姓蘇": -3.94}, "蘇"},
		{"回傳", []string{"json "}, map[string]float64{"回傳雖": -4.93, "回傳j": -3, "回傳js": -2, "回傳jso": -2.5, "回傳json": -2.27}, "json "},
	} {
		s := overwriteSession(tc.table)
		s.documentEnabled, s.neuralLeft = true, tc.left
		if snap := typeContext(s, tc.keys...); snap.Text() != tc.want {
			t.Errorf("%s＋%q：%q，要 %q", tc.left, tc.keys, snap.Text(), tc.want)
		}
	}
}

// 不看神經模型就能排除的：英文字、好幾格重複、同一個鍵連按、大寫字母、前面緊接英數。沒列的字都是 -10，照分數一定會換。
func TestMixedOverwriteStructureKeeps(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"shiningck6", "shining合"},
		{"okgck6", "okg合"},
		{"ggggk6", "什"},
		{"ji3Aqu04", "我A片"},
		{"web3", "web3"},
		{"pi3", "pi3"},
	} {
		s := overwriteSession(map[string]float64{})
		s.documentEnabled, s.neuralLeft = true, "前文"
		if snap := typeContext(s, tc.keys); snap.Text() != tc.want {
			t.Errorf("%s：%q，要 %q", tc.keys, snap.Text(), tc.want)
		}
	}
}

// 沒有神經模型、私人、暫停或沒有前文時維持官方的樣子。
func TestMixedOverwriteNeedsNeural(t *testing.T) {
	table := map[string]float64{"我昨": -2.55, "昨": -2.55}
	for _, tc := range []struct {
		name string
		edit func(s *Session)
		keys []string
		want string
	}{
		{"沒有神經模型", func(s *Session) { s.Rerank = rerank.Options{} }, []string{"ji3", "5jiy6"}, "我5jiy6"},
		{"沒有神經模型（前面多一個聲母）", func(s *Session) { s.Rerank = rerank.Options{} }, []string{"gnj4"}, "g速"},
		{"暫停", func(s *Session) { s.paused = true }, []string{"ji3", "5jiy6"}, "我5jiy6"},
		{"句首沒有前文", func(s *Session) {}, []string{"5jiy6"}, "5jiy6"},
		{"關掉規則", func(s *Session) { s.InputStyle.MixedOverwrite = "" }, []string{"ji3", "5jiy6"}, "我5jiy6"},
	} {
		s := overwriteSession(table)
		tc.edit(s)
		if snap := typeContext(s, tc.keys...); snap.Text() != tc.want {
			t.Errorf("%s：%q，要 %q", tc.name, snap.Text(), tc.want)
		}
	}
	s := overwriteSession(table)
	var snap proto.Snapshot
	for _, k := range "ji35jiy6" {
		snap = s.Handle(proto.Request{Type: proto.Key, Key: string(k), Private: true})
	}
	if snap.Text() != "我5jiy6" {
		t.Errorf("私人：%q", snap.Text())
	}
}

// ↓ 清單不看神經模型也列蓋完的字：待定按鍵（5jiy6）與組字區照字面的一段（gjn␣）。
func TestMixedOverwriteDownOffersOverwritten(t *testing.T) {
	s := overwriteSession(nil)
	typeContext(s, "ji3", "5jiy6")
	if items := itemTexts(key(s, proto.KeyDown)); len(items) < 2 || items[0] != "5jiy6" || items[1] != "昨" {
		t.Fatalf("待定按鍵：%q", items)
	}
	if snap := pickText(t, s, "昨"); snap.Text() != "我昨" {
		t.Fatalf("選了：%q", snap.Text())
	}

	s = overwriteSession(nil)
	typeContext(s, "ji3", "gjn ")
	pickKeys(t, s, 4, "蘇")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我蘇" {
		t.Fatalf("組字區：%q", snap.Commit)
	}
}

// 沒換時用 ↓ 選了蓋完的字再送出就記住，下次同一個位置照記憶換；換了之後選回原按鍵也記住、下次不換。
func TestMixedOverwriteMemory(t *testing.T) {
	s := overwriteSession(nil)
	typeContext(s, "ji3", "5jiy6")
	key(s, proto.KeyDown)
	pickText(t, s, "昨")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我昨" {
		t.Fatalf("送出：%q", snap.Commit)
	}
	want := []personal.MixedToken{{Keys: "5jiy6", Kind: personal.MixedChinese}}
	if rs := memoryRecords(s); len(rs) != 1 || rs[0].MixedRef != (personal.MixedRef{Keys: "5jiy6", Left: "我"}) || !slices.Equal(rs[0].Form, want) {
		t.Fatalf("記住的：%+v", rs)
	}
	if snap := typeContext(s, "ji3", "5jiy6", "wu0 "); snap.Text() != "我昨天" {
		t.Fatalf("再打（照記憶）：%q", snap.Text())
	}

	s = overwriteSession(map[string]float64{"我昨": -2.55})
	if snap := typeContext(s, "ji3", "5jiy6"); snap.Text() != "我昨" {
		t.Fatalf("前提（照神經換）：%q", snap.Text())
	}
	key(s, proto.KeyDown)
	pickText(t, s, "5jiy6")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我5jiy6" {
		t.Fatalf("選回原按鍵送出：%q", snap.Commit)
	}
	if snap := typeContext(s, "ji3", "5jiy6", "wu0 "); snap.Text() != "我5jiy6天" {
		t.Fatalf("再打（照記憶留著）：%q", snap.Text())
	}
}
