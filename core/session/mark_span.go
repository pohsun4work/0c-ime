package session

import (
	"strings"

	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/vocab"
)

var markBaseKey = map[string]string{proto.KeyShiftHome: proto.KeyHome, proto.KeyShiftEnd: proto.KeyEnd}

// normalizeMarkKey 同 normalizeVocabKey：新鍵名在沒有對應功能時就是原本的鍵。
func (s *Session) normalizeMarkKey(k string) string {
	if base, ok := markBaseKey[k]; ok && (s.InputStyle.VocabMark == "" || !s.Preview02 || s.cand != nil) {
		return base
	}
	return k
}

// markKey 處理整合風格（操作規則 vocab_mark）多的標示按鍵（#123）；consumed 為 false 時照 vocabKey 處理：
//   - Shift＋Home／End 同一直按 Shift＋←／→，標到組字區的頭尾（超過詞長上限時標到上限）。
//   - 標示中按 ↓ 換整段：同從標示的開頭按 ↓、再按 Tab 換到標示的長度；Tab 換不到的範圍見 openMarkSpan。
//     游標回到開始標示的位置，選了不動；之後的 Tab 照舊換詞長。
func (s *Session) markKey(k string) (handled, consumed bool) {
	if s.InputStyle.VocabMark == "" {
		return false, false
	}
	switch {
	case k == proto.KeyShiftHome || k == proto.KeyShiftEnd:
		step := proto.KeyShiftRight
		if k == proto.KeyShiftHome {
			step = proto.KeyShiftLeft
		}
		handled, _ = s.vocabKey(step)
		for cursor := -1; handled && cursor != s.cursor; {
			cursor = s.cursor
			handled, _ = s.vocabKey(step)
		}
		return handled, true
	case k == proto.KeyDown && s.mark != nil:
		start, end := s.markRange()
		cursor := s.cursor
		s.mark, s.cursor = nil, s.mark.anchor
		if end-start >= 2 && (s.openAnchored(start, end-start, false) || s.openMarkSpan(start, end)) {
			return true, true
		}
		s.cursor = cursor // 開不了（一個字、沒有按鍵可換）：同其他按鍵，取消標示、照常處理。
	}
	return false, false
}

// openMarkSpan 開 Tab 換不到的標示範圍 [start, end)（中間有分隔的空白、查不到按鍵的字）的整段候選，照跨英數候選
// （openKeySpan）的順序列：空白照留、查不到按鍵的字照畫面、沒記按鍵的中文用讀音的按鍵。
func (s *Session) openMarkSpan(start, end int) bool {
	var units []keyUnit
	keyed := false
	for pos := start; pos < end; {
		u, ok := s.keyUnitAt(pos)
		switch {
		case ok && u.start == pos && u.end <= end && (u.keys != " " || u.end == end):
		case !s.literals[pos]:
			u = keyUnit{start: pos, end: pos + 1, keys: s.layout.Keys(s.syls[pos])}
		default:
			u = keyUnit{start: pos, end: pos + 1, fixed: s.shownText(pos, pos+1)}
		}
		keyed = keyed || u.fixed == ""
		units = append(units, u)
		pos = u.end
	}
	return keyed && s.openUnits(start, end-start, units)
}

// markedEnglish：標示範圍去掉頭尾空白是英文詞（中文與原樣注音的 syls 是注音，不是 vocab.IsEnglish）。
func (s *Session) markedEnglish(start, end int) (string, bool) {
	if s.InputStyle.VocabMark == "" {
		return "", false
	}
	var text strings.Builder
	for _, syl := range s.syls[start:end] {
		text.WriteString(string(syl))
	}
	word := strings.TrimSpace(text.String())
	return word, len(word) >= minComposeWord && vocab.IsEnglish(word)
}

// addMarkedEnglish 把標示的英文詞加入自訂詞彙（沒有讀音的英文詞，#146）。
func (s *Session) addMarkedEnglish(word string) {
	b, err := s.model.VocabAdd([]vocab.Entry{{Text: word}}, vocab.SourceCompose)
	switch {
	case err != nil:
		s.notices = append(s.notices, err.Error())
	case b == nil:
		s.notices = append(s.notices, "「"+word+"」已在詞庫")
	default:
		s.vocabChanged = true
		s.notices = append(s.notices, "已加入詞庫："+word)
	}
}

func (s *Session) markEnglishHint(word string) string {
	if s.model.VocabEnglish(word) {
		return "「" + word + "」已在詞庫　Esc 取消"
	}
	return "Enter 加入英文詞：" + word + "　Esc 取消"
}
