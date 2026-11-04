package session

import (
	"maps"
	"math"
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
)

// wordTable：我＋mp3（允）＋檔，照字面後面的「檔」合理得多（-2 比 -6），字本身 mp3 比 允 低（-7 比 -5）。
var wordTable = map[string]float64{"我允": -5, "我允檔": -6, "我m": -3, "我mp": -2, "我mp3": -2, "我mp3檔": -2}

// alnumWordSession 是 alnumSession 加上 允（ㄩㄣˇ，mp3）、檔、很。
func alnumWordSession(table map[string]float64, extra ...word) *Session {
	return alnumSession(table, append([]word{{"允", "ㄩㄣˇ", 1e-5}, {"檔", "ㄉㄤˇ", 1e-3}, {"很", "ㄏㄣˇ", 1e-3}}, extra...)...)
}

func wordTableWith(more map[string]float64) map[string]float64 {
	out := maps.Clone(wordTable)
	maps.Copy(out, more)
	return out
}

// 官方組成一個中文字的英數按鍵，後面打的字看得出是英數時換回照字面；official 是關掉這條時的樣子（前提）。
func TestMixedAlnumWordNeural(t *testing.T) {
	for _, tc := range []struct {
		name           string
		keys           string
		table          map[string]float64
		extra          []word
		official, want string
	}{
		{"後面的字看得出是英數", "ji3mp32;3", wordTable, nil, "我允檔", "我mp3檔"},
		{"後面的字照中文也合理", "ji3mp32;3", wordTableWith(map[string]float64{"我允檔": -1}), nil, "我允檔", "我允檔"},
		// 官方選的字不合理、同音的別的字合理時（執行續），不拿照字面去比錯字。
		{"同音字更合理", "ji3mp32;3", wordTableWith(map[string]float64{"我隕": -1, "我隕檔": -1}),
			[]word{{"隕", "ㄩㄣˇ", 1e-6}}, "我允檔", "我允檔"},
		// 同音字一次送，有一個字神經模型不認得時整批失敗（NaN），改成一個一個評。
		{"同音字有神經模型不認得的", "ji3mp32;3", wordTableWith(map[string]float64{"我隕": math.NaN()}),
			[]word{{"隕", "ㄩㄣˇ", 1e-6}}, "我允檔", "我mp3檔"},
		// 字本身照字面比較合理也一樣：後面沒有字看不出來（處理器是喔）。
		{"句尾的字", "ji3mp3", wordTableWith(map[string]float64{"我允": -9}), nil, "我允", "我允"},
		{"語言模型看是常見的用法", "ji3mp32;3", wordTable, []word{{"允", "ㄩㄣˇ", .5}}, "我允檔", "我允檔"},
	} {
		s := alnumWordSession(tc.table, tc.extra...)
		withoutAlnum(s)
		if snap := typeContext(s, tc.keys); snap.Text() != tc.official {
			t.Errorf("%s 前提（官方）：%q，要 %q", tc.name, snap.Text(), tc.official)
		}
		s = alnumWordSession(tc.table, tc.extra...)
		if snap := typeContext(s, tc.keys); snap.Text() != tc.want {
			t.Errorf("%s：%q，要 %q", tc.name, snap.Text(), tc.want)
		}
	}
}

// 換過的字，後面改打的字不再支持時換回中文。
func TestMixedAlnumWordBack(t *testing.T) {
	s := alnumWordSession(wordTableWith(map[string]float64{"我允很": -0.5, "我mp3很": -5}))
	if snap := typeContext(s, "ji3mp32;3", proto.KeyBackspace); snap.Text() != "我mp3" {
		t.Fatalf("刪掉後面的字照留：%q", snap.Text())
	}
	if snap := typeContext(s, "cp3"); snap.Text() != "我允很" {
		t.Errorf("改打的字：%q", snap.Text())
	}
}

// ↓ 開換過的字（mp3／允），選回中文送出後，同樣的按鍵照記憶留著中文。
func TestMixedAlnumWordDownAndMemory(t *testing.T) {
	s := alnumWordSession(wordTable)
	typeContext(s, "ji3mp32;3", proto.KeyLeft, proto.KeyLeft)
	if items := itemTexts(key(s, proto.KeyDown)); s.cand.start != 1 || s.cand.length != 3 || !slices.Contains(items, "允") {
		t.Fatalf("開換過的字：start=%d length=%d %q", s.cand.start, s.cand.length, items)
	}
	pickText(t, s, "允")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我允檔" {
		t.Fatalf("送出：%q", snap.Commit)
	}
	if snap := typeContext(s, "ji3mp32;3"); snap.Text() != "我允檔" {
		t.Errorf("再打：%q", snap.Text())
	}
}
