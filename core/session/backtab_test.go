package session

import (
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// 候選開著時 Shift+Tab 是 Tab 的反方向：詞長 Tab 1→2→3→1，Shift+Tab 1→3→2→1；「下一個候選」的 Tab 則往回移一格。
func TestBackTabReversesCandidateTab(t *testing.T) {
	open := func(tab string) *Session {
		s := evidenceSession()
		s.InputStyle.CandidateTab = tab
		for _, w := range []struct{ w, r string }{{"新圖", "ㄒㄧㄣ ㄊㄨˊ"}, {"新圖意", "ㄒㄧㄣ ㄊㄨˊ ㄧˋ"}} {
			s.model.Lex.(*lexicon.Mem).Add(lexicon.Entry{Surface: w.w, Reading: zhuyin.ParseReading(w.r), PRW: 1})
			s.model.LM.(*lm.Mem).SetUni(w.w, .00001)
		}
		typeReadingLocal(s, "ㄒㄧㄣ ㄊㄨˊ ㄧˋ")
		s.Handle(proto.Request{Type: proto.Cursor, To: 0})
		s.Handle(proto.Request{Type: proto.Candidates, Length: 1})
		return s
	}
	lengths := func(s *Session, k string) []int {
		var out []int
		for range 3 {
			snap := key(s, k)
			if snap.Candidates == nil || snap.Commit != "" {
				t.Fatalf("%s 要只換詞長：%+v", k, snap)
			}
			out = append(out, snap.Candidates.Length)
		}
		return out
	}
	if got := lengths(open("length"), proto.KeyTab); !slices.Equal(got, []int{2, 3, 1}) {
		t.Fatalf("Tab 詞長 %v", got)
	}
	if got := lengths(open("length"), proto.KeyBackTab); !slices.Equal(got, []int{3, 2, 1}) {
		t.Fatalf("Shift+Tab 詞長 %v", got)
	}
	s := open("next")
	key(s, proto.KeyTab)
	if snap := key(s, proto.KeyBackTab); snap.Candidates == nil || snap.Candidates.Highlight != 0 {
		t.Fatalf("next：Tab 再 Shift+Tab 要回到第一個：%+v", snap.Candidates)
	}
}
