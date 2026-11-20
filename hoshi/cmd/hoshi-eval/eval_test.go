package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

func TestParseKeys(t *testing.T) {
	got, err := parseKeys("ji3␣{Enter}{pick:它}{Ctrl+,}{Ctrl+[}{KP2}{x}A")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"j", "i", "3", " ", "Enter", "pick:它", "Ctrl+,", "Ctrl+[", "KP2", "{", "x", "}", "A"}
	if !slices.Equal(got, want) {
		t.Fatalf("parseKeys = %q，要 %q", got, want)
	}
	if s := formatKeys(want); s != "ji3␣{Enter}{pick:它}{Ctrl+,}{Ctrl+[}{KP2}{x}A" {
		t.Fatalf("formatKeys = %q", s)
	}
	if _, err := parseKeys("ji3 su3"); err == nil {
		t.Fatal("半形空白要報錯（空白鍵寫 ␣）")
	}
}

func TestTextKeys(t *testing.T) {
	annotate := func(s string) ([]zhuyin.Syllable, bool) {
		m := map[string][]zhuyin.Syllable{"我": {"ㄨㄛˇ"}, "今天": {"ㄐㄧㄣ", "ㄊㄧㄢ"}}
		r, ok := m[s]
		return r, ok
	}
	got, err := textKeys("我 mp3，今天", annotate)
	if err != nil {
		t.Fatal(err)
	}
	if s := formatKeys(got); s != "ji3␣mp3{Ctrl+,}rup␣wu0␣" {
		t.Fatalf("textKeys = %s", s)
	}
	if _, err := textKeys("你", annotate); err == nil {
		t.Fatal("標不出讀音要報錯")
	}
	if _, err := textKeys("「", annotate); err == nil {
		t.Fatal("不知道怎麼打的字元要報錯")
	}
	if _, err := textKeys("a{Enter}", annotate); err == nil {
		t.Fatal("{ 寫成按鍵會被當成鍵名，要報錯")
	}
}

// fakeEngine 把送進來的按鍵照字面放進組字區；Enter 與 commit 送出（前文代號 7），s 鍵回 handled=false。
type fakeEngine struct {
	preedit   string
	reqs      []proto.Request
	cands     []string
	page      int
	fullWidth bool
}

func (f *fakeEngine) handle(req proto.Request) (proto.Snapshot, int64, error) {
	f.reqs = append(f.reqs, req)
	snap := proto.Snapshot{Handled: true, Mode: proto.ModeZhuyin, FullWidth: f.fullWidth}
	switch {
	case req.Type == proto.Select:
		f.preedit, f.cands = f.cands[req.Index], nil
	case req.Type == proto.Commit || req.Type == proto.Key && req.Key == proto.KeyEnter:
		snap.Commit, snap.ContextID, f.preedit = f.preedit, 7, ""
	case req.Type != proto.Key:
	case req.Key == proto.KeyDown:
		f.cands = []string{"甲", "乙", "丙"}
	case req.Key == proto.KeyPageDown:
		f.page++
	case req.Key == "s":
		snap.Handled = false
	default:
		f.preedit += req.Key
	}
	if f.preedit != "" {
		snap.Preedit = []proto.Segment{{Text: f.preedit}}
	}
	if f.cands != nil {
		// 一頁一項，測翻頁
		snap.Candidates = &proto.CandidateList{Items: []proto.CandidateItem{{ID: f.page, Text: f.cands[f.page]}}, Page: f.page, Pages: len(f.cands)}
	}
	return snap, 5, nil
}

func (f *fakeEngine) sent() []string {
	var out []string
	for _, r := range f.reqs {
		if r.Type == proto.Key {
			out = append(out, r.Key)
		} else {
			out = append(out, string(r.Type))
		}
	}
	return out
}

func typeKeys(t *testing.T, sh *shell, keys string) {
	t.Helper()
	ks, err := parseKeys(keys)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range ks {
		if _, err := sh.press(k); err != nil {
			t.Fatal(err)
		}
	}
}

func TestShell(t *testing.T) {
	f := &fakeEngine{}
	sh := newShell(map[string]string{"idle_keys": "ab", "ctrl_punctuation": "true", "ctrl_punctuation_keys": ",.';?!"}, f.handle)
	typeKeys(t, sh, "␣cab{Enter}x{Ctrl+[}{Ctrl+:}{Ctrl+.}a{Ctrl+[}bs{Down}{pick:丙}")
	// 沒在組字：空白、不在 idle_keys 的 c、x 交給應用程式；a 開始組字後 b 也送；Enter 送出 ab。
	// Ctrl+[ 外殼不認得、Ctrl+: 引擎沒列出，都交給應用程式；Ctrl+. 前面是 x 不是 ab，不確認前文。
	// 組字中按引擎不收的 Ctrl+[：先送出組字；b 前面緊接著剛送出的字，先確認前文。
	// s 引擎回未處理，交給應用程式（插在組字區前面）；最後翻兩頁選了「丙」。
	if got := sh.text(); got != " cabxCtrl+.as丙" {
		t.Fatalf("畫面 = %q", got)
	}
	want := []string{"a", "b", "Enter", "Ctrl+.", "a", "commit", "context", "b", "s", "Down", "PageDown", "PageDown", "select"}
	if got := f.sent(); !slices.Equal(got, want) {
		t.Fatalf("送進引擎 %q，要 %q", got, want)
	}
}

func TestShellCaret(t *testing.T) {
	f := &fakeEngine{}
	sh := newShell(map[string]string{"idle_keys": "ab"}, f.handle)
	// 送出 ab 後游標左移：新組字插在 a 後面，前面不是剛送出的字所以不確認前文；End、Backspace 交給應用程式。
	typeKeys(t, sh, "ab{Enter}{Left}a")
	if got := sh.text(); got != "aab" {
		t.Fatalf("畫面 = %q", got)
	}
	typeKeys(t, sh, "{Enter}{End}{Backspace}{Home}{Delete}")
	if got := sh.text(); got != "a" {
		t.Fatalf("畫面 = %q", got)
	}
	if want := []string{"a", "b", "Enter", "a", "Enter"}; !slices.Equal(f.sent(), want) {
		t.Fatalf("送進引擎 %q，要 %q", f.sent(), want)
	}
}

func TestShellFullWidth(t *testing.T) {
	f := &fakeEngine{fullWidth: true}
	sh := newShell(map[string]string{"idle_keys": "ab"}, f.handle)
	if _, err := sh.send(proto.Request{Type: proto.Context, ContextEnabled: true}); err != nil {
		t.Fatal(err)
	}
	// 全形時沒在組字的數字鍵盤、空白、大寫也送進引擎
	typeKeys(t, sh, "{KP2}{Enter}␣{Enter}A")
	if want := []string{"context", "KP2", "Enter", "context", " ", "Enter", "context", "A"}; !slices.Equal(f.sent(), want) {
		t.Fatalf("送進引擎 %q，要 %q", f.sent(), want)
	}
}

// Windows 外殼（keys.rs）認得的 Ctrl＋標點改了，shellCtrlKeys 要跟著改。
func TestShellCtrlKeysMatchWindows(t *testing.T) {
	src, err := os.ReadFile("../../../platforms/windows/tsf/src/keys.rs")
	if err != nil {
		t.Fatal(err)
	}
	fn := regexp.MustCompile(`(?s)fn ctrl_punct_name.*?\n}`).Find(src)
	var got string
	for _, m := range regexp.MustCompile(`"(.)" => "Ctrl\+`).FindAllSubmatch(fn, -1) {
		got += string(m[1])
	}
	if got != shellCtrlKeys {
		t.Fatalf("keys.rs 的 ctrl_punct_name 是 %q，shellCtrlKeys 是 %q", got, shellCtrlKeys)
	}
}

func TestReadCorpus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zh.tsv")
	body := "# 說明\n# profile: default\n# match: baseline\nid\tkeys\twant\tnote\nz1\tji3␣su3\t我␣你\t備註\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := readCorpus(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "zh" || c.Profile != "default" || c.Match != "baseline" || len(c.Cases) != 1 ||
		c.Cases[0] != (Case{ID: "z1", Keys: "ji3␣su3", Want: "我 你", Note: "備註"}) {
		t.Fatalf("readCorpus = %+v", c)
	}
	if err := os.WriteFile(path, []byte(body+"z1\tji3\t我\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCorpus(path); err == nil || !strings.Contains(err.Error(), "重複") {
		t.Fatalf("id 重複要報錯，err = %v", err)
	}
}

func TestCaseMerge(t *testing.T) {
	r := CaseResult{Got: "甲", Steps: []Step{{Sent: true, EngineUS: 9000}, {}}}
	r.merge(CaseResult{Got: "甲", Steps: []Step{{Sent: true, EngineUS: 4000}, {}}})
	r.merge(CaseResult{Got: "乙", Fallback: "fallback", Steps: []Step{{}, {Sent: true, EngineUS: 1}}})
	want := []Step{{Sent: true, EngineUS: 4000}, {}}
	if !slices.Equal(r.Steps, want) || r.Unstable != "乙" || r.Fallback != "fallback" {
		t.Fatalf("merge = %+v", r)
	}
}

func TestRenderReportCompare(t *testing.T) {
	step := func(us int64) Step { return Step{Key: "a", Sent: true, EngineUS: us} }
	base := Run{Label: "官方", Sets: []SetResult{{Name: "mix", Cases: []CaseResult{
		{Case: Case{ID: "1", Want: "甲"}, Got: "甲", Steps: []Step{step(1000)}},
		{Case: Case{ID: "2", Want: "乙"}, Got: "x", Steps: []Step{step(1000)}},
	}}}}
	cur := Run{Label: "fork", Sets: []SetResult{{Name: "mix", Cases: []CaseResult{
		{Case: Case{ID: "1", Want: "甲"}, Got: "y", Steps: []Step{step(45000)}},
		{Case: Case{ID: "2", Want: "乙"}, Got: "乙", Steps: []Step{step(1000)}},
	}}}}
	md := renderReport(cur, &base)
	for _, want := range []string{"| mix | 2 | 1 | 50.0% | 50.0% | 1 | 1 | 2 | 0 | 0 |", "| 44.0 | 1 |", "mix：變差 1 句", "mix：變好 1 句"} {
		if !strings.Contains(md, want) {
			t.Errorf("報告裡沒有 %q：\n%s", want, md)
		}
	}
}
