package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/fuzzy"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

func testEngine() *Engine {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, w := range []string{"吃", "痴"} {
		lex.Add(lexicon.Entry{Surface: w, Reading: zhuyin.ParseReading("ㄔ"), PRW: 1})
		sys.SetUni(w, 1e-3)
	}
	return New(lex, sys)
}

// 宿主設定的模糊音交給之後開的每個 session；缺少設定時全部關閉。
func TestOpenSessionCarriesFuzzyConfig(t *testing.T) {
	e := testEngine()
	if e.Session(e.OpenSession()).Fuzzy.Enabled() {
		t.Fatal("預設不得啟用任何模糊音")
	}
	e.Fuzzy = fuzzy.Config{}.With(fuzzy.CCh, fuzzy.Select)
	id := e.OpenSession()
	if got := e.Session(id).Fuzzy; got.Key() != "c-ch=select" {
		t.Fatalf("session 的模糊音設定：%q", got.Key())
	}
}

// 宿主設定的尾端前綴快取上限（#307，手機設 512）交給之後開的每個 session；沒設時照預設。
func TestOpenSessionCarriesPrefixCacheKeys(t *testing.T) {
	e := testEngine()
	if got := e.Session(e.OpenSession()).Opt.PrefixCacheKeys; got != 0 {
		t.Fatalf("沒設時應照預設（0）：%d", got)
	}
	e.PrefixCacheKeys = 512
	if got := e.Session(e.OpenSession()).Opt.PrefixCacheKeys; got != 512 {
		t.Fatalf("session 沒有帶到快取上限：%d", got)
	}
}

// #183：外殼查詢的閒置按鍵：1 是 ㄅ；= 是明確打的符號，留在組字區（fork）。
func TestIdleKeysIncludePhoneticAndSymbolKeys(t *testing.T) {
	e := testEngine()
	if got := e.IdleKeys(); !strings.Contains(got, "1") || !strings.Contains(got, "=") {
		t.Fatalf("1 是 ㄅ、= 是符號：%q", got)
	}
}

// containsAll 回報 keys 是否包含 want 的每個字元。
func containsAll(keys, want string) bool {
	for _, r := range want {
		if !strings.ContainsRune(keys, r) {
			return false
		}
	}
	return true
}

// #133：使用者開啟「選字後游標移到下一個字」時，之後開的每個 session 都照設定；預設關閉。
func TestOpenSessionCarriesAdvanceAfterSelect(t *testing.T) {
	e := testEngine()
	if e.Session(e.OpenSession()).AdvanceAfterSelect {
		t.Fatal("預設選字後游標不動")
	}
	e.AdvanceAfterSelect = true
	if !e.Session(e.OpenSession()).AdvanceAfterSelect {
		t.Fatal("session 沒有帶到選字後游標後移的設定")
	}
}

// #112：使用者選的選字鍵帶到之後開的每個 session；預設空字串（1234567890）。
func TestOpenSessionCarriesSelectionKeys(t *testing.T) {
	e := testEngine()
	if e.Session(e.OpenSession()).SelectionKeys != "" {
		t.Fatal("預設 1234567890")
	}
	e.SelectionKeys = "1234qweras"
	if e.Session(e.OpenSession()).SelectionKeys != "1234qweras" {
		t.Fatal("session 沒有帶到選字鍵")
	}
}

// 以位元組協定走一遍：近音候選的附註出現在 JSON 快照裡；原音候選沒有 note 欄位。
func TestFuzzyNoteSurvivesTheByteProtocol(t *testing.T) {
	e := testEngine()
	e.Fuzzy = fuzzy.Config{}.With(fuzzy.CCh, fuzzy.Select)
	id := e.OpenSession()
	// ㄘ＋一聲：用注音鍵（symbol）打，混打不會把 h␣ 當英文。
	for _, req := range []string{`{"type":"symbol","symbol":"ㄘ"}`, `{"type":"key","key":" "}`} {
		if _, err := e.Handle(id, []byte(req)); err != nil {
			t.Fatal(err)
		}
	}
	out, err := e.Handle(id, []byte(`{"type":"candidates","length":1}`))
	if err != nil {
		t.Fatal(err)
	}
	var snap proto.Snapshot
	if err := json.Unmarshal(out, &snap); err != nil || snap.Candidates == nil || len(snap.Candidates.Items) < 2 {
		t.Fatalf("快照：%s %v", out, err)
	}
	items := snap.Candidates.Items
	if items[0].Note != "近音 ㄔ" || items[len(items)-1].Note != "" {
		t.Fatalf("附註：%+v", items)
	}
	var raw struct {
		Candidates struct {
			Items []map[string]any `json:"items"`
		} `json:"candidates"`
	}
	json.Unmarshal(out, &raw)
	last := raw.Candidates.Items[len(raw.Candidates.Items)-1]
	if _, has := last["note"]; has {
		t.Fatalf("沒有附註時不應輸出 note 欄位：%v", last)
	}
}

// #115：全部共用時，一個輸入框切到英文，其他輸入框（含新開的）下一個請求就是英文；外殼換輸入框時送的
// 「設成中文」改成回到共用的模式；macOS Caps Lock 的「設成英文」只影響那個輸入框。
func TestSharedModeAcrossSessions(t *testing.T) {
	e := testEngine()
	e.SharedMode = true
	a, b := e.OpenSession(), e.OpenSession()
	mode := func(id uint64, r proto.Request) string {
		snap, err := e.Do(id, r)
		if err != nil {
			t.Fatal(err)
		}
		return snap.Mode
	}
	if mode(a, proto.Request{Type: proto.Toggle}) != proto.ModeEnglish {
		t.Fatal("切到英文")
	}
	if mode(b, proto.Request{Type: proto.Key, Key: "a"}) != proto.ModeEnglish {
		t.Fatal("另一個輸入框跟上")
	}
	c := e.OpenSession()
	if mode(c, proto.Request{Type: proto.Mode, On: false}) != proto.ModeEnglish {
		t.Fatal("新輸入框的焦點重設回到共用的模式（英文）")
	}
	if mode(b, proto.Request{Type: proto.Toggle}) != proto.ModeZhuyin || mode(a, proto.Request{Type: proto.Key, Key: "j"}) != proto.ModeZhuyin {
		t.Fatal("切回注音也同步")
	}
	if mode(a, proto.Request{Type: proto.Mode, On: true}) != proto.ModeEnglish || mode(b, proto.Request{Type: proto.Key, Key: "j"}) != proto.ModeZhuyin {
		t.Fatal("Caps Lock 的設成英文只影響那個輸入框")
	}
	// 沒有開全部共用：各自的模式，照外殼原本的做法。
	e2 := testEngine()
	x, y := e2.OpenSession(), e2.OpenSession()
	e2.Do(x, proto.Request{Type: proto.Toggle})
	if snap, _ := e2.Do(y, proto.Request{Type: proto.Key, Key: "j"}); snap.Mode != proto.ModeZhuyin {
		t.Fatal("各自的模式")
	}
}
