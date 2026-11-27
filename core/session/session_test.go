package session

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

func newSession() *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range []struct {
		w, r string
		p    float64
	}{
		{"我", "ㄨㄛˇ", 1e-2}, {"想", "ㄒㄧㄤˇ", 3e-3}, {"吃", "ㄔ", 2e-3},
		{"景", "ㄐㄧㄥˇ", 1e-3}, {"井", "ㄐㄧㄥˇ", 3e-4}, {"風", "ㄈㄥ", 6e-4}, {"風景", "ㄈㄥ ㄐㄧㄥˇ", 2e-4},
		{"嘛", "ㄇㄚˊ", 5e-4}, {"麻煩", "ㄇㄚˊ ㄈㄢˊ", 3e-4}, // ㄈㄢˊ 沒有單字：只能靠「麻煩」到達
	} {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, e.p)
	}
	return New(personal.NewModel(lex, sys))
}

func typeSyl(s *Session, syl string) proto.Snapshot {
	var snap proto.Snapshot
	for _, k := range layout.Standard.Keys(zhuyin.Syllable(syl)) {
		snap = s.Handle(proto.Request{Type: proto.Key, Key: string(k)})
	}
	return snap
}

// symbolSyl 用注音鍵（symbol 請求）打一個音節，不經混打的英數判斷：測試詞庫只收少數字，
// 混打會把詞庫裡沒有或罕用的音節照字面留下（真實詞庫每個音節都有字），測近音、解碼本身的測試用這個。
func symbolSyl(s *Session, syl string) proto.Snapshot {
	text := []rune(syl)
	if zhuyin.ClassOf(text[len(text)-1]) != zhuyin.Tone {
		text = append(text, zhuyin.Tone1)
	}
	var snap proto.Snapshot
	for _, r := range text {
		snap = s.Handle(proto.Request{Type: proto.Symbol, Symbol: string(r)})
	}
	return snap
}

func key(s *Session, k string) proto.Snapshot {
	return s.Handle(proto.Request{Type: proto.Key, Key: k})
}

func TestIdleKeysGoBackToTheApp(t *testing.T) {
	s := newSession()
	for _, k := range []string{proto.KeyEnter, proto.KeyTab, proto.KeyEscape, proto.KeyLeft, proto.KeyDown, " ", "Control"} {
		if key(s, k).Handled {
			t.Errorf("閒置時 %q 應交還應用程式", k)
		}
	}
	if !key(s, "j").Handled {
		t.Error("注音鍵應被處理")
	}
}

func TestInsertAndDeleteInTheMiddle(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄨㄛˇ")
	typeSyl(s, "ㄔ")
	s.Handle(proto.Request{Type: proto.Cursor, To: 1})
	snap := typeSyl(s, "ㄒㄧㄤˇ")
	if snap.Text() != "我想吃" || snap.Cursor.Rune != 2 || snap.Cursor.UTF16 != 2 {
		t.Fatalf("中段插入：%q 游標 %+v", snap.Text(), snap.Cursor)
	}
	snap = key(s, proto.KeyBackspace)
	if snap.Text() != "我吃" || snap.Cursor.Rune != 1 {
		t.Fatalf("中段刪除：%q 游標 %+v", snap.Text(), snap.Cursor)
	}
}

func TestPendingSymbolsShowAtCursor(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄨㄛˇ")
	typeSyl(s, "ㄔ")
	s.Handle(proto.Request{Type: proto.Cursor, To: 1})
	snap := key(s, "v") // ㄒ
	if snap.Text() != "我ㄒ吃" || snap.Preedit[1].Style != proto.StylePending || snap.Cursor.Rune != 2 {
		t.Fatalf("未完成符號應顯示在游標處：%+v 游標 %+v", snap.Preedit, snap.Cursor)
	}
	// 音節打到一半時退格只刪符號，不刪前面的字
	if snap = key(s, proto.KeyBackspace); snap.Text() != "我吃" {
		t.Fatalf("退格應只移除符號：%q", snap.Text())
	}
}

func TestLocksFollowEdits(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄨㄛˇ")
	typeSyl(s, "ㄐㄧㄥˇ")
	s.Handle(proto.Request{Type: proto.Candidates, Length: 1})
	snap := key(s, "2") // 候選開啟時數字鍵選字：第 2 項是 井
	if snap.Text() != "我井" || snap.Candidates != nil {
		t.Fatalf("數字鍵選字：%q", snap.Text())
	}
	// 在鎖定之前插入：鎖定右移，字不變
	s.Handle(proto.Request{Type: proto.Cursor, To: 1})
	snap = typeSyl(s, "ㄒㄧㄤˇ")
	if snap.Text() != "我想井" || snap.Preedit[len(snap.Preedit)-1].Style != proto.StyleLocked {
		t.Fatalf("鎖定應跟著平移：%+v", snap.Preedit)
	}
	// 刪掉被鎖定的音節：鎖定解除
	s.Handle(proto.Request{Type: proto.Cursor, To: 3})
	key(s, proto.KeyBackspace)
	if snap = typeSyl(s, "ㄐㄧㄥˇ"); snap.Text() != "我想景" {
		t.Fatalf("鎖定應已解除：%q", snap.Text())
	}
}

func TestCandidateWindowSwallowsKeys(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄐㄧㄥˇ")
	key(s, proto.KeyDown)
	snap := key(s, "j") // 注音鍵在候選開啟時不得同時被解讀
	if !snap.Handled || snap.Text() != "景" || snap.Candidates == nil {
		t.Fatalf("候選開啟時應吃掉按鍵且不改變組字：%+v", snap)
	}
	if snap = key(s, proto.KeyEscape); snap.Candidates != nil || snap.Text() != "景" {
		t.Fatal("Esc 應只關閉候選、保留組字")
	}
	if snap = key(s, proto.KeyEscape); snap.Text() != "" {
		t.Fatal("第二次 Esc 應放棄組字")
	}
}

// 使用者回報：選字時上下鍵不能移動，只能按數字，像是卡死。
func TestCandidateHighlightMovesWithArrows(t *testing.T) {
	s := newSession()
	symbolSyl(s, "ㄐㄧㄥˇ") // 注音鍵打：清單不含混打的原始按鍵

	snap := key(s, proto.KeyDown)
	at := func(want int, text string) {
		t.Helper()
		c := snap.Candidates
		if c == nil || c.Highlight != want || c.Items[c.Highlight].Text != text {
			t.Fatalf("反白應在第 %d 項「%s」：%+v", want, text, c)
		}
	}
	at(0, "景")
	snap = key(s, proto.KeyDown)
	at(1, "井")
	snap = key(s, proto.KeyDown)
	at(2, "ㄐㄧㄥˇ")                // 漢字候選之後可選原樣注音
	snap = key(s, proto.KeyDown) // 只有一種詞長：走過尾端回到開頭，不停在原地
	at(0, "景")
	snap = key(s, proto.KeyUp)
	at(2, "ㄐㄧㄥˇ")
	snap = key(s, proto.KeyUp)
	at(1, "井")
	if snap = key(s, proto.KeyEnter); snap.Candidates != nil || snap.Text() != "井" || snap.Commit != "" {
		t.Fatalf("Enter 應選取反白的候選而不提交：%+v", snap)
	}
}

// P8：↓ 先開游標後面那一個字；上下鍵在同一份清單裡繞圈、不換詞長，換詞長用 Tab（官方各詞長接成一份清單）。
func TestCandidateHighlightStaysInWordLength(t *testing.T) {
	s := newSession()
	symbolSyl(s, "ㄈㄥ") // 注音鍵打：清單不含混打的原始按鍵
	symbolSyl(s, "ㄐㄧㄥˇ")
	s.Handle(proto.Request{Type: proto.Cursor, To: 0})
	snap := key(s, proto.KeyDown)
	if c := snap.Candidates; c == nil || c.Length != 1 || c.Items[c.Highlight].Text != "風" {
		t.Fatalf("應先開游標後面的字：%+v", c)
	}
	if c := key(s, proto.KeyUp).Candidates; c == nil || c.Length != 1 || c.Highlight != len(c.Items)-1 {
		t.Fatalf("往上應繞到同一份清單的最後一個：%+v", c)
	}
	if c := key(s, proto.KeyDown).Candidates; c == nil || c.Length != 1 || c.Items[c.Highlight].Text != "風" {
		t.Fatalf("往下應繞回第一個：%+v", c)
	}
	if c := key(s, proto.KeyTab).Candidates; c == nil || c.Length != 2 || c.Items[c.Highlight].Text != "風景" {
		t.Fatalf("Tab 換成較長的詞：%+v", c)
	}
}

func TestCommitExactlyOnce(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄈㄥ")
	typeSyl(s, "ㄐㄧㄥˇ")
	snap := key(s, proto.KeyEnter)
	if snap.Commit != "風景" || snap.Text() != "" || !snap.Handled {
		t.Fatalf("提交：%+v", snap)
	}
	if snap = key(s, proto.KeyEnter); snap.Commit != "" || snap.Handled {
		t.Fatal("已無組字時 Enter 應交還應用程式，且不得再次提交")
	}
}

func TestPauseLearning(t *testing.T) {
	s := newSession()
	s.Handle(proto.Request{Type: proto.Pause, On: true})
	typeSyl(s, "ㄨㄛˇ")
	key(s, proto.KeyEnter)
	if s.model.Store.Total != 0 {
		t.Fatal("暫停學習時不應有任何學習紀錄")
	}
	s.Handle(proto.Request{Type: proto.Pause})
	typeSyl(s, "ㄨㄛˇ")
	key(s, proto.KeyEnter)
	if s.model.Store.Total == 0 {
		t.Fatal("恢復後應學習")
	}
}

func TestInvalidSelectIsReported(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄐㄧㄥˇ")
	s.Handle(proto.Request{Type: proto.Candidates})
	if snap := s.Handle(proto.Request{Type: proto.Select, Index: 99}); len(snap.Notices) == 0 {
		t.Fatal("無效的選取應回報")
	}
}

func TestPunctuation(t *testing.T) {
	s := newSession()
	if snap := key(s, "<"); !snap.Handled || snap.Commit != "" || snap.Text() != "<" {
		t.Fatalf("標點應留在組字區：%+v", snap)
	}
	if snap := key(s, proto.KeyEnter); snap.Commit != "<" || snap.Text() != "" {
		t.Fatalf("Enter 才提交標點：%+v", snap)
	}
	typeSyl(s, "ㄨㄛˇ")
	if snap := key(s, ">"); !snap.Handled || snap.Commit != "" || snap.Text() != "我>" {
		t.Fatalf("組字與標點一起保留：%+v", snap)
	}
	if snap := key(s, proto.KeyEnter); snap.Commit != "我>" || snap.Text() != "" {
		t.Fatalf("提交應與顯示相同：%+v", snap)
	}
}

func TestVisibleCharactersStayInPreedit(t *testing.T) {
	for _, k := range []string{"3", "4", "6", "7", "A", "@", "#"} {
		t.Run(k, func(t *testing.T) {
			s := newSession()
			typeSyl(s, "ㄨㄛˇ")
			typeSyl(s, "ㄔ")
			key(s, proto.KeyLeft)
			snap := key(s, k)
			if !snap.Handled || snap.Commit != "" || snap.Text() != "我"+k+"吃" || snap.Cursor.Rune != 2 {
				t.Fatalf("字元應留在組字游標處：%+v", snap)
			}
			if snap = key(s, proto.KeyBackspace); !snap.Handled || snap.Text() != "我吃" {
				t.Fatalf("誤按字元應可單獨刪除：%+v", snap)
			}
		})
	}
}

func TestToggleEnglishMode(t *testing.T) {
	for _, mode := range []proto.Request{{Type: proto.Toggle}, {Type: proto.Mode, On: true}} {
		s := newSession()
		typeSyl(s, "ㄨㄛˇ")
		typeSyl(s, "ㄔ")
		key(s, proto.KeyLeft)
		key(s, proto.KeyDown)
		snap := s.Handle(mode)
		if snap.Commit != "" || snap.Text() != "我吃" || snap.Cursor.Rune != 1 || snap.Mode != proto.ModeEnglish || snap.Candidates != nil {
			t.Fatalf("切到英文應保留組字與游標並關閉候選：%+v", snap)
		}
		text := ""
		for _, k := range "aZ1234 <`@" {
			text += string(k)
			snap = key(s, string(k))
			if !snap.Handled || snap.Commit != "" || snap.Text() != "我"+text+"吃" || snap.Cursor.Rune != 1+len([]rune(text)) || snap.Candidates != nil {
				t.Fatalf("英文模式應在游標處原樣插入：%+v", snap)
			}
		}
		key(s, proto.KeyLeft)
		if snap = key(s, proto.KeyBackspace); snap.Text() != "我aZ1234 <@吃" {
			t.Fatalf("英文模式可移動並單獨刪除：%+v", snap)
		}
		if snap = s.Handle(proto.Request{Type: proto.Toggle}); snap.Mode != proto.ModeZhuyin || snap.Commit != "" {
			t.Fatalf("切回注音不應提交：%+v", snap)
		}
		if snap = typeSyl(s, "ㄒㄧㄤˇ"); snap.Text() != "我aZ1234 <想@吃" {
			t.Fatalf("切回注音可在原位置繼續組字：%+v", snap)
		}
		s.Handle(proto.Request{Type: proto.Mode, On: true})
		if snap = key(s, proto.KeyEnter); !snap.Handled || snap.Commit != "我aZ1234 <想@吃" || snap.Text() != "" {
			t.Fatalf("英文模式 Enter 應提交所見混合文字：%+v", snap)
		}
		for _, k := range []string{"j", "1", "<", " ", proto.KeyBackspace, proto.KeyLeft, proto.KeyEnter} {
			if snap = key(s, k); snap.Handled || snap.Mode != proto.ModeEnglish {
				t.Fatalf("英文模式無組字時 %q 應交還應用程式：%+v", k, snap)
			}
		}
		s.Handle(proto.Request{Type: proto.Mode})
		if !key(s, "j").Handled {
			t.Fatal("Mode 不帶 On 應設回注音")
		}
	}
}

// #90：切到英文清掉句中未打完的注音，已組好的字與游標保留；之後的英數接在游標處。
func TestEnglishClearsPendingZhuyin(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄨㄛˇ")
	typeSyl(s, "ㄔ")
	key(s, proto.KeyLeft)
	key(s, "v") // 尚未完成的 ㄒ
	snap := s.Handle(proto.Request{Type: proto.Toggle})
	if snap.Text() != "我吃" || snap.Commit != "" || !s.comp.Empty() || snap.Cursor.Rune != 1 || snap.Mode != proto.ModeEnglish {
		t.Fatalf("切到英文應清掉未打完的注音、保留已組好的字與游標：%+v", snap)
	}
	if snap := key(s, "1"); snap.Text() != "我1吃" || snap.Commit != "" || snap.Cursor.Rune != 2 {
		t.Fatalf("英數輸入應接在游標處：%+v", snap)
	}
	if snap := key(s, proto.KeyBackspace); snap.Text() != "我吃" {
		t.Fatalf("退格應只刪誤按數字：%+v", snap)
	}
	if snap := s.Handle(proto.Request{Type: proto.Toggle, Restore: true}); snap.Text() != "我吃" || !s.comp.Empty() {
		t.Fatalf("切到英文後有過操作，切回注音不放回清掉的注音：%+v", snap)
	}
	s.Handle(proto.Request{Type: proto.Toggle})
	if snap := key(s, proto.KeyEscape); !snap.Handled || snap.Text() != "" || snap.Commit != "" {
		t.Fatalf("英文模式 Esc 可取消組字：%+v", snap)
	}
}

func TestEnglishHidesChineseSuggestions(t *testing.T) {
	s := newSession()
	s.model.LM.(*lm.Mem).SetUni("麻煩", 0.01)
	if snap := typeSyl(s, "ㄇㄚˊ"); snap.Suggestion != "麻煩" {
		t.Fatalf("需有中文補全：%+v", snap)
	}
	if snap := s.Handle(proto.Request{Type: proto.Toggle}); snap.Suggestion != "" {
		t.Fatalf("英文模式不應顯示中文補全：%+v", snap)
	}
	if snap := key(s, proto.KeyTab); !snap.Handled || snap.Commit != "" || snap.Text() != "麻" { // 組字中吃掉不動作（#106）
		t.Fatalf("英文 Tab 不應接受中文補全：%+v", snap)
	}
	if snap := s.Handle(proto.Request{Type: proto.Toggle}); snap.Suggestion != "麻煩" {
		t.Fatalf("切回注音應恢復補全：%+v", snap)
	}
}

func TestCommitPreservesPendingPosition(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄨㄛˇ")
	typeSyl(s, "ㄔ")
	key(s, proto.KeyLeft)
	key(s, "v")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我ㄒ吃" || snap.Text() != "" {
		t.Fatalf("提交不能把句中尚未完成注音移到句尾：%+v", snap)
	}
}

// 隨機操作（打字、亂按數字鍵、選字、移動游標、退格）之後，已輸入的音節一定顯示得出來：
// 每個音節、未打完的注音、混打暫存的按鍵都至少一字。任何操作序列都不能讓組字區空白。
// 混打照字面的英數與選過的英文詞一段可以多字，所以只檢查下限。
func TestPreeditNeverVanishes(t *testing.T) {
	syls := []string{"ㄨㄛˇ", "ㄒㄧㄤˇ", "ㄔ", "ㄐㄧㄥˇ", "ㄈㄥ", "ㄇㄚˊ", "ㄈㄢˊ"}
	keys := []string{proto.KeyBackspace, proto.KeyBackspace, proto.KeyLeft, proto.KeyRight, proto.KeyUp, proto.KeyDown,
		proto.KeyEnter, proto.KeyEscape, proto.KeyTab, " ", "1", "2", "3", "j", "u", "[", "]", "<", "`", ".", "^", "_"}
	seeds := int64(3000)
	if testing.Short() {
		seeds = 300
	}
	for seed := range seeds {
		r := rand.New(rand.NewSource(seed))
		s := newSession()
		var trace []string
		for range 30 {
			var snap proto.Snapshot
			if r.Intn(8) == 0 {
				restore := r.Intn(2) == 0
				trace = append(trace, fmt.Sprintf("Toggle(restore=%v)", restore))
				snap = s.Handle(proto.Request{Type: proto.Toggle, Restore: restore})
			} else if r.Intn(3) == 0 {
				y := syls[r.Intn(len(syls))]
				trace = append(trace, y)
				snap = typeSyl(s, y)
			} else {
				k := keys[r.Intn(len(keys))]
				trace = append(trace, k)
				snap = key(s, k)
			}
			if snap.Commit != "" {
				continue
			}
			minChars := len(s.syls) + len([]rune(s.comp.String()))
			if s.mixed.raw != "" {
				minChars++
			}
			if got := len([]rune(snap.Text())); got < minChars {
				t.Fatalf("seed %d：應顯示至少 %d 個字元，組字區卻是 %q\n操作：%q", seed, minChars, snap.Text(), trace)
			}
		}
	}
}
