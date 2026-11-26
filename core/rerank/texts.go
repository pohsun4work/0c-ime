package rerank

import (
	"errors"
	"time"
	"unicode/utf8"
)

// ModeTexts 是中英混打的切法比較（P7）：候選是長短不一、可含英數的完整文字，評分器回傳整串 logP(文字｜前文)，
// 不看讀音。全部文字在同一次評分裡算完。
const ModeTexts = "texts"

// MaxTexts 與 MaxTextRunes 同 worker（sequence_runtime.py 的 MAX_TEXTS、MAX_TEXT_RUNES）。
const (
	MaxTexts     = 64
	MaxTextRunes = 32
)

// ErrDeadline：評分器沒有在期限內算完。結果不套用，照規則；評分器自己算完，不中斷。
var ErrDeadline = errors.New("texts scoring deadline exceeded")

// TextsRequest 只含前文（同 Request.Context 的契約）與要比較的文字。
type TextsRequest struct {
	Revision int      `json:"revision"`
	Context  string   `json:"context,omitempty"`
	Texts    []string `json:"texts"`
}

// TextsScorer 是支援 ModeTexts 的評分器。deadline 之前沒算完回 ErrDeadline。
type TextsScorer interface {
	ScoreTexts(r TextsRequest, deadline time.Duration) ([]float64, error)
}

// FitsText：t 能放進 texts 評分：非空、不超過 MaxTextRunes 字、每個字都在字表內（Encodable；評分器遇到表外字會整批拒絕）。
func (o Options) FitsText(t string) bool {
	n := utf8.RuneCountInString(t)
	return n > 0 && n <= MaxTextRunes && utf8.ValidString(t) && o.Encodable(t)
}

// SupportsTexts 是送出前的檢查：2～MaxTexts 個不重複、FitsText 的文字，前文不超過 MaxContextRunes 字。
func (o Options) SupportsTexts(r TextsRequest) bool {
	if len(r.Texts) < 2 || len(r.Texts) > MaxTexts || !utf8.ValidString(r.Context) || utf8.RuneCountInString(r.Context) > MaxContextRunes {
		return false
	}
	seen := make(map[string]bool, len(r.Texts))
	for _, t := range r.Texts {
		if seen[t] || !o.FitsText(t) {
			return false
		}
		seen[t] = true
	}
	return true
}
