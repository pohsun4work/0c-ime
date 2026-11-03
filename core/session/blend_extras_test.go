package session

import (
	"testing"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/proto"
)

// 全域開關逐條作用：關掉的那一條照官方，其他照常。↓／Tab 跨過英數只管手動換，關掉時自動的規則照樣作用。
func TestBlendExtrasSwitchRulesOneByOne(t *testing.T) {
	without := func(f func(*inputstyle.Extras)) inputstyle.Extras {
		x := inputstyle.AllExtras()
		f(&x)
		return x
	}
	noResplit := without(func(x *inputstyle.Extras) { x.MixedResplit = false })
	noRepeats := without(func(x *inputstyle.Extras) { x.MixedRepeats = false })
	noReversed := without(func(x *inputstyle.Extras) { x.MixedReversed = false })
	noLiteral := without(func(x *inputstyle.Extras) { x.MixedLiteral = false })
	noZhuyin := without(func(x *inputstyle.Extras) { x.MixedZhuyinText = false })
	p, _ := inputstyle.Get("blend")
	for _, tc := range []struct {
		keys  []string
		extra inputstyle.Extras
		full  bool // 跟全部開著一樣；false 是跟官方一樣
	}{
		{[]string{"ggggk6"}, noRepeats, false},
		{[]string{"ggggk6"}, noResplit, true},
		{[]string{"gck6"}, noRepeats, true},
		{[]string{"gck6"}, noResplit, true},
		{[]string{"url "}, noReversed, false},
		{[]string{"url "}, noLiteral, true},
		{[]string{"url "}, noResplit, true},
		{[]string{"npm "}, noLiteral, false},
		{[]string{"npm "}, noReversed, true},
		{[]string{"npm "}, noResplit, true},
		{[]string{"e", "KP2", "e"}, noLiteral, false},
		{[]string{"e", "KP2", "e"}, noResplit, true},
		{[]string{"ji3a "}, noZhuyin, false},
		{[]string{"ji3a "}, noResplit, true},
		{[]string{"ji3a "}, noLiteral, true},
	} {
		official, full := typeContext(contextSession(false), tc.keys...).Text(), typeContext(contextSession(true), tc.keys...).Text()
		if official == full {
			t.Fatalf("%q：官方與全部開著一樣（%q），測不出開關", tc.keys, full)
		}
		s := contextSession(true)
		s.InputStyle = tc.extra.Apply(p.Rules)
		want := official
		if tc.full {
			want = full
		}
		if got := typeContext(s, tc.keys...).Text(); got != want {
			t.Errorf("%q %+v：%q，要 %q", tc.keys, tc.extra, got, want)
		}
	}
}

// 英文模式打的字不算混打：倒退鍵把留著的注音收回組字（不是待定按鍵），接著打的英文在注音後面；
// 英文模式打的字母不跟後面的中文合成重複鍵。
func TestBlendKeyRecordsSkipEnglishMode(t *testing.T) {
	toggle := func(s *Session) { s.Handle(proto.Request{Type: proto.Toggle}) }
	s := contextSession(true)
	typeContext(s, "ji3", "su", "KP1")
	toggle(s)
	if snap := keys(s, proto.KeyBackspace, "a", "b"); snap.Text() != "我ㄋㄧab" {
		t.Errorf("英文模式倒退再打：%q", snap.Text())
	}
	s = contextSession(true)
	typeContext(s, "ji3")
	toggle(s)
	key(s, "g")
	toggle(s)
	if snap := typeContext(s, "gk6"); snap.Text() != "我g什" {
		t.Errorf("英文模式的 g：%q", snap.Text())
	}
}
