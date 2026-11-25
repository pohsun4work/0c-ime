package session

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// #109、#107 第 2 點：沒有字可讀、由聲調鍵（含空白＝一聲）完成的音節，連同聲調原樣留在組字區。

func syllableLiteralSession(extra ...word) *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range append([]word{
		{"我", "ㄨㄛˇ", 1e-2}, {"想", "ㄒㄧㄤˇ", 3e-3}, {"吃", "ㄔ", 2e-3}, {"欠", "ㄑㄧㄢˋ", 1e-3}, {"喔", "ㄛ", 1e-3},
	}, extra...) {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, e.p)
	}
	return New(personal.NewModel(lex, sys))
}

func keys(s *Session, ks ...string) proto.Snapshot {
	var snap proto.Snapshot
	for _, k := range ks {
		snap = key(s, k)
	}
	return snap
}

// 使用者資料不得留下原樣注音（不學習、不進個人資料）。
func assertNoZhuyinLearned(t *testing.T, s *Session, literal string) {
	t.Helper()
	b, err := json.Marshal(s.model.Store)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(string(b), literal) {
		t.Fatalf("原樣注音不得進個人資料：%s", b)
	}
}

// 中英混打：既有混打規則優先。開頭的數字鍵＋空白維持數字；組不成中文的聲調按鍵照字面顯示原按鍵，
// ↓ 另提供帶聲調的原樣注音。
func TestMixedInputSyllableLiteral(t *testing.T) {
	s := syllableLiteralSession()
	if snap := keys(s, "1", " "); snap.Text() != "1 " {
		t.Fatalf("開頭數字鍵＋空白維持數字：%q", snap.Text())
	}
	key(s, proto.KeyEscape)

	s = syllableLiteralSession()
	if snap := keys(s, "c", "o", "6"); snap.Text() != "co6" {
		t.Fatalf("混打照字面顯示原按鍵：%q", snap.Text())
	}
	snap := key(s, proto.KeyDown)
	id := -1
	for _, it := range snap.Candidates.Items {
		if it.Text == "ㄏㄟˊ" {
			id = it.ID
		}
	}
	if id < 0 {
		t.Fatalf("↓ 應提供帶聲調的原樣注音：%+v", snap.Candidates.Items)
	}
	s.Handle(proto.Request{Type: proto.Select, Index: id})
	if snap = typeSyl(s, "ㄑㄧㄢˋ"); snap.Text() != "ㄏㄟˊ欠" {
		t.Fatalf("選入後接著打中文：%q", snap.Text())
	}
	if snap = key(s, proto.KeyEnter); snap.Commit != "ㄏㄟˊ欠" {
		t.Fatalf("提交 %q", snap.Commit)
	}
	assertNoZhuyinLearned(t, s, "ㄏㄟˊ")
}

// #107 客戶回報：ㄟ 自己沒有字，詞庫只有「工口」讀 ㄟ ㄌㄨㄛˊ。ㄟ＋空白曾顯示成「工」（更長的詞的開頭），
// 接著打 ㄋㄧˇ 接不上，結果卡成「工ㄋㄧˇ」。只能當詞頭、自己沒有字的音節同樣原樣留下注音，
// 下一個音節能和它成詞時再收回解碼。
func prefixOnlySession() *Session {
	return syllableLiteralSession(
		word{"你", "ㄋㄧˇ", 5e-3}, word{"欸", "ㄟˋ", 1e-4}, word{"工口", "ㄟ ㄌㄨㄛˊ", 1e-5},
		word{"子", "ㄗˇ", 1e-3}, word{"妮子", "ㄋㄧ ㄗ˙", 1e-5}, word{"妮維雅", "ㄋㄧ ㄨㄟˊ ㄧㄚˇ", 1e-5},
		word{"維", "ㄨㄟˊ", 1e-4},
	)
}

func TestPrefixOnlySyllableStaysZhuyin(t *testing.T) {
	s := prefixOnlySession()
	snap := keys(s, "o", " ")
	if snap.Text() != "ㄟ" || !s.comp.Empty() || len(s.literals) != 1 {
		t.Fatalf("ㄟ＋空白原樣留下注音，不顯示成「工」：%q 暫存 %q", snap.Text(), s.comp.String())
	}
	if snap = typeSyl(s, "ㄋㄧˇ"); snap.Text() != "ㄟ你" {
		t.Fatalf("接著打的 ㄋㄧˇ 自成一個字，不併進前一個音節也不卡成原樣注音：%q", snap.Text())
	}
	if snap = key(s, proto.KeyEnter); snap.Commit != "ㄟ你" {
		t.Fatalf("提交 %q", snap.Commit)
	}
	assertNoZhuyinLearned(t, s, "ㄟ")

	// 不按空白直接打聲母：同一個音節照一般注音規則組成 ㄋㄟ（各家注音輸入法相同），不受影響。
	s = prefixOnlySession()
	if snap = keys(s, "o", "s"); snap.Text() != "ㄋㄟ" {
		t.Fatalf("同一音節內的組音規則不變：%q", snap.Text())
	}
}

func TestPrefixOnlySyllableJoinsFollowingWord(t *testing.T) {
	for _, c := range []struct{ syls, want string }{
		{"ㄟ ㄌㄨㄛˊ", "工口"},
		{"ㄋㄧ ㄗ˙", "妮子"},
		{"ㄋㄧ ㄨㄟˊ ㄧㄚˇ", "妮維雅"},
	} {
		s := prefixOnlySession()
		var snap proto.Snapshot
		for _, y := range strings.Fields(c.syls) {
			snap = typeSyl(s, y)
		}
		if snap.Text() != c.want || len(s.literals) != 0 {
			t.Fatalf("%s：下一個音節能成詞時收回解碼：%q 字面 %v", c.syls, snap.Text(), s.literals)
		}
	}
	// 打到一半（還只是詞頭）時也看得到：ㄋㄧ＋ㄨㄟˊ 顯示「妮維」的前綴。
	s := prefixOnlySession()
	typeSyl(s, "ㄋㄧ")
	if snap := typeSyl(s, "ㄨㄟˊ"); snap.Text() != "妮維" || len(s.literals) != 0 {
		t.Fatalf("詞頭兩音節收回解碼：%q", snap.Text())
	}
}

// 使用者從 ↓ 明確選入的原樣音節是字面文字，下一個音節不得把它收回去變成漢字。
func TestChosenSyllableLiteralIsNotJoined(t *testing.T) {
	s := prefixOnlySession()
	keys(s, "o", " ")
	snap := key(s, proto.KeyDown)
	id := -1
	for _, it := range snap.Candidates.Items {
		if it.Text == "ㄟˋ" {
			id = it.ID
		}
	}
	if id < 0 {
		t.Fatalf("↓ 應提供其他聲調：%+v", snap.Candidates.Items)
	}
	if snap = s.Handle(proto.Request{Type: proto.Select, Index: id}); snap.Text() != "ㄟˋ" {
		t.Fatalf("選入原樣 ㄟˋ：%q", snap.Text())
	}
	if snap = typeSyl(s, "ㄋㄧˇ"); snap.Text() != "ㄟˋ你" {
		t.Fatalf("明確選的原樣音節維持字面：%q", snap.Text())
	}
}

// 中英混打同樣：o＋空白是 ㄟ，接著 su3 是「你」，不是 ㄟㄋㄧˇ 的保底注音；詞頭接得上時照樣成詞。
func TestMixedInputPrefixOnlySyllable(t *testing.T) {
	s := prefixOnlySession()
	if snap := keys(s, "o", " "); snap.Text() != "ㄟ" {
		t.Fatalf("混打 ㄟ＋空白原樣留下注音：%q", snap.Text())
	}
	if snap := keys(s, "s", "u", "3"); snap.Text() != "ㄟ你" {
		t.Fatalf("混打接著打的 ㄋㄧˇ 成字：%q", snap.Text())
	}
	if snap := key(s, proto.KeyEnter); snap.Commit != "ㄟ你" {
		t.Fatalf("提交 %q", snap.Commit)
	}

	s = prefixOnlySession()
	if snap := keys(s, "s", "u", " ", "y", "7"); snap.Text() != "妮子" {
		t.Fatalf("混打詞頭接得上時收回成詞：%q", snap.Text())
	}
}
