package session

import (
	"testing"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

func homophoneSession() *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, x := range []struct {
		w, r string
		p    float64
	}{
		{"小", "ㄒㄧㄠˇ", .01}, {"龍", "ㄌㄨㄥˊ", .01}, {"小龍", "ㄒㄧㄠˇ ㄌㄨㄥˊ", .6},
		{"小籠包", "ㄒㄧㄠˇ ㄌㄨㄥˊ ㄅㄠ", .1}, {"包", "ㄅㄠ", .01}, {"我", "ㄨㄛˇ", .1},
		{"麻", "ㄇㄚˊ", .01}, {"婆", "ㄆㄛˊ", .01}, {"麻婆豆腐", "ㄇㄚˊ ㄆㄛˊ ㄉㄡˋ ㄈㄨˇ", .6},
	} {
		lex.Add(lexicon.Entry{Surface: x.w, Reading: zhuyin.ParseReading(x.r), PRW: 1})
		sys.SetUni(x.w, x.p)
	}
	return New(personal.NewModel(lex, sys))
}

func typeXiaolong(s *Session) proto.Snapshot {
	typeSyl(s, "ㄒㄧㄠˇ")
	return typeSyl(s, "ㄌㄨㄥˊ")
}

// 麻婆 是 麻婆豆腐 的開頭，小視窗自己跳〔麻婆豆腐〕，Tab 接受。
func typeMapo(s *Session) proto.Snapshot {
	typeSyl(s, "ㄇㄚˊ")
	return typeSyl(s, "ㄆㄛˊ")
}

// 沒有小視窗時 Tab 吃掉不動作：不會把畫面上的 小龍 換成同音開頭的 小籠包。
func TestTabWithoutPopupKeepsHomophones(t *testing.T) {
	s := homophoneSession()
	if snap := typeXiaolong(s); snap.Text() != "小龍" || snap.Suggestion != "" {
		t.Fatalf("原組字不應被改寫：%+v", snap)
	}
	if snap := key(s, proto.KeyTab); !snap.Handled || snap.Text() != "小龍" || snap.Suggestion != "" {
		t.Fatalf("Tab 不動：%+v", snap)
	}
	if got := key(s, proto.KeyEnter).Commit; got != "小龍" {
		t.Fatal(got)
	}
}

func TestCompletionPreservesEarlierLockAndDoesNotLearnPrivateOrPaused(t *testing.T) {
	for _, mode := range []string{"private", "pause"} {
		s := homophoneSession()
		if mode == "pause" {
			s.Handle(proto.Request{Type: proto.Pause, On: true})
		}
		typeSyl(s, "ㄨㄛˇ")
		s.locks[0] = decode.Lock{Word: "我", Len: 1}
		typeMapo(s)
		if mode == "private" {
			s.Handle(proto.Request{Type: proto.Close, Private: true})
		}
		snap := key(s, proto.KeyTab)
		if snap.Text() != "我麻婆豆腐" || s.locks[0].Word != "我" {
			t.Fatal(snap, s.locks)
		}
		key(s, proto.KeyEnter)
		if s.model.Store.Total != 0 {
			t.Fatalf("%s 不應學習", mode)
		}
	}
}

func TestCommittedTextDoesNotLeakIntoAnotherComposition(t *testing.T) {
	s := homophoneSession()
	typeMapo(s)
	key(s, proto.KeyTab)
	s.Handle(proto.Request{Type: proto.Pause, On: true})
	key(s, proto.KeyEnter)
	other := homophoneSession()
	a, b := typeMapo(s), typeMapo(other)
	if a.Text() != b.Text() || a.Suggestion != b.Suggestion || a.SuggestionEdit == nil || b.SuggestionEdit == nil {
		t.Fatal(a, b)
	}
}
