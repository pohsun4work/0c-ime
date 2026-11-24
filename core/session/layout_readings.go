package session

import (
	"github.com/0c-biggo/0c-ime/core/input"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// expand 是組字的音節格：使用者打的音，加上簡拼的讀音。
func (s *Session) expand(syls []zhuyin.Syllable) input.Lattice {
	lat := s.widenSpatial(syls, s.front.Expand(syls))
	for i, y := range syls {
		if !s.literals[i] && y.Abbreviated() {
			lat[i] = s.topAbbreviated(y, lat[i])
		}
	}
	return lat
}
