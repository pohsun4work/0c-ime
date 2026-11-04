package session

import (
	"slices"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/rerank"
)

// alnumSession 是整合風格的混打 session，詞庫多了英數後面常接的詞，英文字典是 arm、i、url；table 同 overwriteSession（沒列的字 -10）。
func alnumSession(table map[string]float64) *Session {
	s := overwriteSession(table,
		word{"製", "ㄓˋ", 1e-3}, word{"程", "ㄔㄥˊ", 1e-3}, word{"製程", "ㄓˋ ㄔㄥˊ", 1e-3},
		word{"螢", "ㄧㄥˊ", 1e-4}, word{"幕", "ㄇㄨˋ", 1e-4}, word{"木", "ㄇㄨˋ", 1e-3}, word{"螢幕", "ㄧㄥˊ ㄇㄨˋ", 1e-3},
		word{"名", "ㄇㄧㄥˊ", 1e-3}, word{"目", "ㄇㄨˋ", 1e-3}, word{"局", "ㄐㄩˊ", 1e-3},
		word{"手", "ㄕㄡˇ", 1e-3}, word{"機", "ㄐㄧ", 1e-3}, word{"手機", "ㄕㄡˇ ㄐㄧ", 1e-3},
		word{"網", "ㄨㄤˇ", 1e-3}, word{"路", "ㄌㄨˋ", 1e-3}, word{"網路", "ㄨㄤˇ ㄌㄨˋ", 1e-3}, word{"爽", "ㄕㄨㄤˇ", 1e-3},
		word{"高", "ㄍㄠ", 1e-3}, word{"滿", "ㄇㄢˇ", 1e-3}, word{"之", "ㄓ", 1e-3}, word{"顯", "ㄒㄧㄢˇ", 1e-3}, word{"交", "ㄐㄧㄠ", 1e-3},
	)
	s.English = english.New([]english.Word{{Text: "arm", Weight: 3}, {Text: "i", Weight: 3}, {Text: "url", Weight: 3}})
	return s
}

// withoutAlnum 關掉英數接中文，是官方（加上其他 mixed_context 規則）的樣子。
func withoutAlnum(s *Session) {
	rules := strings.Split(s.InputStyle.MixedContext, ",")
	s.InputStyle.MixedContext = strings.Join(slices.DeleteFunc(rules, func(c string) bool { return c == inputstyle.ContextAlnum }), ",")
}

// 神經模型評畫面上的字，換成的切法明顯合理才換；official 是關掉這條時的樣子（前提）。
func TestMixedAlnumNeural(t *testing.T) {
	for _, tc := range []struct {
		name           string
		keys           []string
		table          map[string]float64
		official, want string
	}{
		{"數字當成帳號、整串照字面", []string{"3nm54t/6"},
			map[string]float64{"3n": -1, "3nm": -1, "3nm製": -2.5, "3nm製程": -0.5}, "3nm54t/6", "3nm製程"},
		{"前段只有一個字母", []string{"4ku/6aj4"},
			map[string]float64{"4k": -2, "4k螢": -3, "4k螢幕": -0.5}, "4ku/6木", "4k螢幕"},
		{"官方切錯位置", []string{"5gj;3xj4"},
			map[string]float64{"5g": -2, "5g網": -3, "5g網路": -0.5, "5爽": -6, "5爽路": -5}, "5爽路", "5g網路"},
		{"官方把空白當成一聲", []string{"i5 vu03"},
			map[string]float64{"i之": -4, "i之顯": -5, "i5": -2, "i5 ": -1, "i5 顯": -2}, "i之顯", "i5 顯"},
		// 差不到門檻照官方：lisa03 照字面 -9.08、lis滿 -8.49（2026-11-04 本機實測）。
		{"差不到門檻", []string{"lisa03"},
			map[string]float64{"li": -1, "lis": -1, "lisa": -2, "lisa0": -3, "lisa03": -2.08, "lis滿": -6.49}, "lisa03", "lisa03"},
		{"官方的切法差不到門檻", []string{"4ku/6aj4"},
			map[string]float64{"4k": -2, "4ku": -2, "4ku/": -2, "4ku/6": -2, "4ku/6木": -1, "4k螢": -4, "4k螢幕": -2}, "4ku/6木", "4ku/6木"},
		// 換過的一串，後面打的字讓別的切法更合理時門檻較低：2f名 接著打 幕 是 2fa螢幕（-9 比 2f名目 -10）。
		{"換過的一串再換", []string{"2fau/6", "aj4"},
			map[string]float64{"2f": -2, "2f名": -3, "2fa": -2, "2fa螢": -4, "2f名目": -5, "2f名木": -5, "2fa螢幕": -1}, "2fau/6木", "2fa螢幕"},
		{"換過的一串換回照字面", []string{"arm6", "4"},
			map[string]float64{"ar": -3, "arm": -3, "arm6": -3, "a局": -5, "arm64": -0.5}, "arm64", "arm64"},
	} {
		s := alnumSession(tc.table)
		withoutAlnum(s)
		if snap := typeContext(s, tc.keys...); snap.Text() != tc.official {
			t.Errorf("%s 前提（官方）：%q，要 %q", tc.name, snap.Text(), tc.official)
		}
		s = alnumSession(tc.table)
		if snap := typeContext(s, tc.keys...); snap.Text() != tc.want {
			t.Errorf("%s：%q，要 %q", tc.name, snap.Text(), tc.want)
		}
	}
	s := alnumSession(map[string]float64{"ar": -3, "arm": -3, "arm6": -3, "a局": -5})
	if snap := typeContext(s, "arm6"); snap.Text() != "a局" {
		t.Errorf("arm6 先換成 a局：%q", snap.Text())
	}
}

// 不看神經模型就照官方的：英數後面接空白（盤古之白、一聲）、只有一個全是數字鍵的音節（帳號）。沒列的字都是 -10，照分數一定會換。
func TestMixedAlnumStructureKeeps(t *testing.T) {
	for _, keys := range []string{"4k ", "4kel ", "4k aj4", "kevin87"} {
		s := alnumSession(map[string]float64{})
		withoutAlnum(s)
		official := typeContext(s, keys).Text()
		s = alnumSession(map[string]float64{})
		if snap := typeContext(s, keys); snap.Text() != official {
			t.Errorf("%q：%q，要照官方 %q", keys, snap.Text(), official)
		}
	}
}

// 換過的一段 mixed_context 的規則不改（4g＋手機 的 g 不併進「手」），接著打的字照常改（重複鍵 ggggk6 是「什」、
// 反序打的 url␣ 不是「交」）。
func TestMixedAlnumGuardsRules(t *testing.T) {
	s := alnumSession(map[string]float64{})
	if snap := typeContext(s, "4gg.3ru "); snap.Text() != "4g手機" {
		t.Errorf("%q", snap.Text())
	}
	s = alnumSession(map[string]float64{"4k": -2, "4k螢": -3, "4k螢幕": -0.5})
	if snap := typeContext(s, "4ku/6aj4", "ggggk6"); snap.Text() != "4k螢幕什" {
		t.Errorf("接著打的重複鍵：%q", snap.Text())
	}
	s = alnumSession(map[string]float64{"4k": -2, "4k螢": -3, "4k螢幕": -0.5})
	if snap := typeContext(s, "4ku/6aj4", "url "); snap.Text() != "4k螢幕url " {
		t.Errorf("接著打的反序英文字：%q", snap.Text())
	}
}

// 換過的一串後面接著打的另一串自己重看；游標移到一串中間時不看。
func TestMixedAlnumNextStretch(t *testing.T) {
	s := alnumSession(map[string]float64{"4k": -2, "4k螢": -3, "4k螢幕": -0.5,
		"4k螢幕5g": -2, "4k螢幕5g網": -3, "4k螢幕5g網路": -0.5, "4k螢幕5爽": -6, "4k螢幕5爽路": -5})
	if snap := typeContext(s, "4ku/6aj4", "5gj;3xj4"); snap.Text() != "4k螢幕5g網路" {
		t.Errorf("接著打的另一串：%q", snap.Text())
	}
	s = alnumSession(map[string]float64{"3n": -1, "3nm": -1, "3nm製": -2.5, "3nm製程": -0.5})
	rules := s.InputStyle.MixedContext
	withoutAlnum(s)
	typeContext(s, "3nm54t/6", "ji3")
	s.InputStyle.MixedContext = rules
	if snap := key(s, proto.KeyLeft); snap.Text() != "3nm54t/6我" {
		t.Errorf("游標移到中間：%q", snap.Text())
	}
}

// 沒有神經模型、私人、暫停時照官方。
func TestMixedAlnumNeedsNeural(t *testing.T) {
	table := map[string]float64{"3n": -1, "3nm": -1, "3nm製": -2.5, "3nm製程": -0.5}
	for _, tc := range []struct {
		name string
		edit func(s *Session)
	}{
		{"沒有神經模型", func(s *Session) { s.Rerank = rerank.Options{} }},
		{"暫停", func(s *Session) { s.paused = true }},
		{"關掉規則", withoutAlnum},
	} {
		s := alnumSession(table)
		tc.edit(s)
		if snap := typeContext(s, "3nm54t/6"); snap.Text() != "3nm54t/6" {
			t.Errorf("%s：%q", tc.name, snap.Text())
		}
	}
	s := alnumSession(table)
	var snap proto.Snapshot
	for _, k := range "3nm54t/6" {
		snap = s.Handle(proto.Request{Type: proto.Key, Key: string(k), Private: true})
	}
	if snap.Text() != "3nm54t/6" {
		t.Errorf("私人：%q", snap.Text())
	}
}

// ↓ 開換過的整段（連同換了之後接著打的字），選回原按鍵送出後，同樣的按鍵照記憶留著，前一個字不同也一樣。
func TestMixedAlnumDownAndMemory(t *testing.T) {
	for _, tc := range []struct {
		name               string
		keys, rest, before string // before：換的當下（打完 keys）畫面上的樣子，rest 是接著打的
		table              map[string]float64
		want               string
		length             int
	}{
		{"待定按鍵", "3nm54t/6", "", "3nm製程", map[string]float64{"3n": -1, "3nm": -1, "3nm製": -2.5, "3nm製程": -0.5}, "3nm製程", 5},
		{"接著打的字", "4ku/6", "aj4", "4k螢", map[string]float64{"4k": -2, "4k螢": -3, "4k螢幕": -0.5}, "4k螢幕", 4},
	} {
		s := alnumSession(tc.table)
		if snap := typeContext(s, tc.keys); snap.Text() != tc.before {
			t.Fatalf("%s 換的當下：%q", tc.name, snap.Text())
		}
		if snap := typeContext(s, tc.rest); tc.rest != "" && snap.Text() != tc.want {
			t.Fatalf("%s：%q", tc.name, snap.Text())
		}
		keys := tc.keys + tc.rest
		if items := itemTexts(key(s, proto.KeyDown)); s.cand.start != 0 || s.cand.length != tc.length || len(items) < 2 || items[0] != tc.want || !slices.Contains(items, keys) {
			t.Fatalf("%s 整段：start=%d length=%d %q", tc.name, s.cand.start, s.cand.length, items)
		}
		pickText(t, s, keys)
		if snap := key(s, proto.KeyEnter); snap.Commit != keys {
			t.Fatalf("%s 送出：%q", tc.name, snap.Commit)
		}
		for _, again := range []struct{ left, want string }{{"", keys}, {"ji3", "我" + keys}} {
			if snap := typeContext(s, again.left+keys); snap.Text() != again.want {
				t.Errorf("%s 再打（前面 %q）：%q", tc.name, again.left, snap.Text())
			}
			key(s, proto.KeyEnter)
		}
	}
}

// Shift＋Delete 忘掉蓋過英數接中文的記憶：換回 4k螢幕，↓ 照樣開整段。
func TestMixedAlnumForgetMemory(t *testing.T) {
	s := alnumSession(map[string]float64{"4k": -2, "4k螢": -3, "4k螢幕": -0.5})
	s.Preview02 = true
	typeContext(s, "4ku/6aj4")
	key(s, proto.KeyDown)
	pickText(t, s, "4ku/6aj4")
	key(s, proto.KeyEnter)
	if snap := typeContext(s, "4ku/6aj4"); snap.Text() != "4ku/6aj4" {
		t.Fatalf("前提：照記憶留著：%q", snap.Text())
	}
	key(s, proto.KeyDown)
	if snap := key(s, proto.KeyShiftDelete); snap.Text() != "4k螢幕" || !snap.Forgot {
		t.Fatalf("Shift＋Delete：%q forgot=%v", snap.Text(), snap.Forgot)
	}
	if items := itemTexts(key(s, proto.KeyDown)); s.cand.start != 0 || s.cand.length != 4 || len(items) < 2 || items[0] != "4k螢幕" {
		t.Errorf("整段：start=%d length=%d %q", s.cand.start, s.cand.length, items)
	}
}
