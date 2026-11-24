package session

import (
	"slices"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
)

func toneMarkSession() *Session {
	return syllableLiteralSession(word{"個", "ㄍㄜˋ", 2e-3})
}

func TestToneMarkLeavesMixedInputAlone(t *testing.T) {
	s := toneMarkSession()
	snap := keys(s, "3", " ")
	if strings.ContainsAny(snap.Text(), "ˇ") || !strings.HasPrefix(snap.Text(), "3") {
		t.Fatalf("中英混打照舊（#110／#134）：%q", snap.Text())
	}
	// 英文模式沒在組字時照舊交還應用程式。
	s = toneMarkSession()
	s.Handle(proto.Request{Type: proto.Mode, On: true})
	if key(s, "3").Handled {
		t.Fatal("英文模式閒置時數字交還應用程式")
	}
}

// #153：中英混打時落單的聲調鍵是數字（#78、#110、#134），↓ 的第一個候選仍是數字，
// 第二個給這個鍵的聲調符號，讓混打的人也能單獨打出 ˊ。前後緊接著數字、英文的聲調鍵是數字或英數的一部分，不加聲調符號。
func downItems(snap proto.Snapshot) []string {
	if snap.Candidates == nil {
		return nil
	}
	var out []string
	for _, item := range snap.Candidates.Items {
		out = append(out, item.Text)
	}
	return out
}

func hasToneMark(items []string) bool {
	return slices.ContainsFunc(items, func(t string) bool { return strings.ContainsAny(t, "ˇˋˊ˙") })
}

func TestMixedLoneToneKeyDownOffersToneMarkSecond(t *testing.T) {
	for k, want := range map[string]string{"3": "ˇ", "4": "ˋ", "6": "ˊ", "7": "˙"} {
		// 句首。
		s := toneMarkSession()
		if snap := key(s, k); snap.Text() != k {
			t.Fatalf("混打 %s 照舊顯示數字：%+v", k, snap)
		}
		items := downItems(key(s, proto.KeyDown))
		if len(items) < 2 || items[0] != k || items[1] != want {
			t.Fatalf("混打句首 %s 按 ↓ 應為 %s、%s：%q", k, k, want, items)
		}
		snap := s.Handle(proto.Request{Type: proto.Select, Index: 1})
		if snap.Text() != want || snap.Candidates != nil {
			t.Fatalf("選聲調符號：%+v", snap)
		}
		if snap = key(s, proto.KeyEnter); snap.Commit != want {
			t.Fatalf("提交 %q", snap.Commit)
		}
		assertNoZhuyinLearned(t, s, want)

		// 句中：接在中文後面。
		s = toneMarkSession()
		typeSyl(s, "ㄨㄛˇ")
		key(s, k)
		items = downItems(key(s, proto.KeyDown))
		if len(items) < 2 || items[0] != k || items[1] != want {
			t.Fatalf("混打句中 %s 按 ↓：%q", k, items)
		}
		if snap = s.Handle(proto.Request{Type: proto.Select, Index: 1}); snap.Text() != "我"+want {
			t.Fatalf("句中選聲調符號：%q", snap.Text())
		}
		// 不選、直接 Esc 關掉：數字照舊。
		s = toneMarkSession()
		keys(s, k, proto.KeyDown, proto.KeyEscape)
		if snap = key(s, proto.KeyEnter); snap.Commit != k {
			t.Fatalf("↓ 後 Esc 再 Enter 照舊送出數字：%q", snap.Commit)
		}
	}

	// 數字的一部分：前後緊接著數字的聲調鍵不加聲調符號。
	for _, run := range []string{"36", "63", "33"} {
		s := toneMarkSession()
		for _, k := range run {
			key(s, string(k))
		}
		if items := downItems(key(s, proto.KeyDown)); hasToneMark(items) {
			t.Fatalf("%s 是數字，↓ 不加聲調符號：%q", run, items)
		}
	}
}

// repeatToneSession 收連按聲調鍵（#321）會用到的字。
func repeatToneSession() *Session {
	return syllableLiteralSession(word{"好", "ㄏㄠˇ", 3e-3}, word{"對", "ㄉㄨㄟˋ", 3e-3}, word{"嘛", "ㄇㄚˊ", 1e-3},
		word{"嗎", "ㄇㄚ˙", 3e-3}, word{"個", "ㄍㄜˋ", 2e-3})
}

func TestRepeatedToneKeyStaysNumberInMixedInput(t *testing.T) {
	// 中英混打：連按的數字與「有3個」（u.33ek4）分不出來，照舊是數字。
	s := repeatToneSession()
	typeSyl(s, "ㄏㄠˇ")
	if snap := key(s, "3"); snap.Text() != "好3" {
		t.Fatalf("中英混打照舊：%q", snap.Text())
	}
}
