package session

import (
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/proto"
)

// 顯示門檻是保守的工程初值，不是排序參數：常用為系統詞頻至少萬分之一，
// 個人常用為有效使用證據至少 4，關聯為鄰詞使系統機率至少提高 2 倍。
// 每列只給一個標示，優先個人常用、關聯、常用；不替排名或模型推測捏造來源。
const (
	commonLabelProbability = 1e-4
	personalLabelEvidence  = 4
	relatedLabelLift       = 2
)

func (s *Session) candidateLabels(c *candState) []string {
	labels := make([]string, len(c.items))
	if c.literal {
		return labels
	}
	// 只用選取範圍外、完整且相鄰的已顯示詞；不跨標點、不拿被替換的詞當前文。
	var left, right, documentLeft string
	documentWeight := s.Opt.DocumentContextWeight
	for _, g := range s.path {
		if g.Partial || s.literals[g.Start] {
			continue
		}
		if g.End == c.start {
			left = g.Word
		}
		if g.Start == c.start+c.length {
			right = g.Word
		}
	}
	if c.start == 0 && documentWeight > 0 && documentWeight <= 1 && !s.DocumentContextOff {
		if word := s.contextLeft(0); word != lm.BOS {
			documentLeft = word
		}
	}
	for i, candidate := range c.items {
		if len(candidate.Entry.Reading) == 0 || i < len(c.matched) && c.matched[i] != "" {
			continue // 原樣注音、符號及近音候選保留原提示
		}
		word := candidate.Entry.Surface
		if !s.private && !s.paused && s.model.WordEvidence(word) >= personalLabelEvidence {
			labels[i] = proto.CandidatePersonal
			continue
		}
		base := s.model.SystemProb(lm.BOS, word)
		context := base
		if left != "" {
			context = s.model.SystemProb(left, word)
		} else if documentLeft != "" {
			context = (1-documentWeight)*base + documentWeight*s.model.SystemProb(documentLeft, word)
		}
		related := base > 0 && context >= relatedLabelLift*base
		if right != "" {
			rightBase := s.model.SystemProb(lm.BOS, right)
			related = related || rightBase > 0 && s.model.SystemProb(word, right) >= relatedLabelLift*rightBase
		}
		switch {
		case related:
			labels[i] = proto.CandidateRelated
		case s.model.LM.Uni(word) >= commonLabelProbability:
			labels[i] = proto.CandidateCommon
		}
	}
	return labels
}
