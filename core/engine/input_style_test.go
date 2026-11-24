package engine

import (
	"testing"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/proto"
)

// fork：微軟的半形原字元沒在組字時也留在組字區（官方交還應用程式），TestKeyDown 要吃；` 打 `。
func TestMicrosoftIdleKeysClaimRawHalfwidthPunctuation(t *testing.T) {
	e := testEngine()
	if !containsAll(e.IdleKeys(), "\\'\"<>?`") {
		t.Fatalf("TestKeyDown must claim the raw halfwidth keys: %q", e.IdleKeys())
	}
}

func styleDo(t *testing.T, e *Engine, id uint64, r proto.Request) proto.Snapshot {
	t.Helper()
	s, err := e.Do(id, r)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStyleWidthIsolatedAcrossEngineSessions(t *testing.T) {
	e := testEngine()
	e.InputStyle.ShiftSpace = inputstyle.ShiftSpaceFullWidth
	e.SharedMode = true
	e.StartEnglish = true
	a, b := e.OpenSession(), e.OpenSession()
	styleDo(t, e, a, proto.Request{Type: proto.Mode, On: true})
	styleDo(t, e, a, proto.Request{Type: proto.Key, Key: proto.KeyShiftSpace})
	if snap := styleDo(t, e, a, proto.Request{Type: proto.Key, Key: "a"}); snap.Commit != "ａ" || !snap.FullWidth || snap.Mode != proto.ModeEnglish {
		t.Fatalf("wide session: %+v", snap)
	}
	if snap := styleDo(t, e, b, proto.Request{Type: proto.Key, Key: "a"}); snap.Handled || snap.FullWidth || snap.Mode != proto.ModeEnglish {
		t.Fatalf("width leaked through shared language mode: %+v", snap)
	}
	styleDo(t, e, a, proto.Request{Type: proto.Cancel})
	if snap := styleDo(t, e, a, proto.Request{Type: proto.Key, Key: "1"}); snap.Commit != "１" || !snap.FullWidth || snap.Mode != proto.ModeEnglish {
		t.Fatalf("reset lost width or mode: %+v", snap)
	}
	styleDo(t, e, a, proto.Request{Type: proto.Toggle})
	styleDo(t, e, a, proto.Request{Type: proto.Key, Key: "t"})
	styleDo(t, e, a, proto.Request{Type: proto.Key, Key: " "})
	if snap := styleDo(t, e, a, proto.Request{Type: proto.Commit}); snap.Commit != "吃" || snap.Mode != proto.ModeZhuyin || !snap.FullWidth {
		t.Fatalf("focus commit changed mode or width: %+v", snap)
	}
	e.CloseSession(a)
	c := e.OpenSession()
	if snap := styleDo(t, e, c, proto.Request{Type: proto.Display, Columns: 1}); snap.FullWidth {
		t.Fatalf("new session inherited old width: %+v", snap)
	}
}

func TestStyleHorizontalCandidateColumnsSurviveDisplayNegotiation(t *testing.T) {
	for _, size := range []int{9, 10} {
		e := testEngine()
		e.InputStyle = inputstyle.Rules{CandidateNavigation: inputstyle.NavigationHorizontal, CandidatePageSize: size}
		e.CandidateColumns = 10
		id := e.OpenSession()
		styleDo(t, e, id, proto.Request{Type: proto.Display, Columns: 10})
		styleDo(t, e, id, proto.Request{Type: proto.Key, Key: "t"})
		styleDo(t, e, id, proto.Request{Type: proto.Key, Key: " "})
		snap := styleDo(t, e, id, proto.Request{Type: proto.Candidates})
		if snap.Candidates == nil || snap.Candidates.Columns != size {
			t.Fatalf("horizontal page %d: %+v", size, snap)
		}
		// UI reports the available width, including a restored horizontal row.
		styleDo(t, e, id, proto.Request{Type: proto.Display, Columns: 5})
		snap = styleDo(t, e, id, proto.Request{Type: proto.Display, Columns: 10})
		if snap.Candidates.Columns != size {
			t.Fatalf("display still clamped to grid: %+v", snap)
		}
		snap = styleDo(t, e, id, proto.Request{Type: proto.Key, Key: proto.KeyRight})
		if snap.Candidates.Highlight != 1 {
			t.Fatalf("horizontal navigation changed: %+v", snap)
		}
	}
}
