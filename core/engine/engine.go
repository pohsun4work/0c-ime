// Package engine 是核心的頂層：共用的唯讀資料、個人化模型與 session 工廠。
// Handle 的形狀（位元組進、位元組出）與 C ABI、行程間協定相同（docs/19）。
package engine

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/fuzzy"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/punct"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/session"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

type Engine struct {
	InputStyle inputstyle.Rules
	mu         sync.Mutex // 個人化資料只有一個寫入者
	Model      *personal.Model
	// 由宿主在建立 session 前設定；nil 使用核心預設值。
	// 資料包可攜帶其開發集校準的前文權重，不寫入個人詞庫。
	DocumentContextWeight *float64
	Rerank                rerank.Options   // 宿主在建立 session 前設定；可選、同步、有界評分器。
	Punct                 punct.Punctuator // 宿主在建立 session 前設定；提交時補標點，nil 停用。
	DocumentContextOff    bool             // 使用者關閉前文時由宿主設定；外殼的前文確認一律忽略。
	EscKeepComposition    bool             // 使用者關閉「按 Esc 清除整段組字」時由宿主設定。
	AdvanceAfterSelect    bool             // 使用者開啟「選字後游標移到下一個字」時由宿主設定（#133）。
	SelectionKeys         string           // 使用者選的選字鍵（#112）；空字串為 1234567890。
	NoLearning            bool             // 「記住我選的字詞」關閉時由宿主設定（#297）；選字、神經選字照常，只是不寫進個人資料。
	Fuzzy                 fuzzy.Config     // 使用者逐對啟用的模糊音；零值全部關閉。
	EnglishAutocomplete   bool
	English               *english.Dictionary // nil uses the authored prototype vocabulary.
	EnglishPack           string              // host-supplied pack version for diagnostics; empty means prototype.
	CandidateBeforeCursor bool
	CandidateColumns      int
	Preview02             bool // 0.2.0 的功能（#128）；0.2.0 起宿主一律打開，零值時行為與沒有這些功能時相同（測試用）
	// SharedMode：所有輸入框共用同一個中英模式（#115，settings 的 mode_memory=shared）。一個輸入框切換後，
	// 其他輸入框在下一個請求時跟上；零值照外殼原本的做法（每個 session 各自的模式）。
	SharedMode bool

	// PrefixCacheKeys：每個 session 的解碼器尾端前綴快取最多幾個讀音（#307），宿主在建立 session 前設定。
	// 0 為預設 4096（桌面）；手機設較小的值壓住記憶體。只影響速度與記憶體，不影響選字結果。
	PrefixCacheKeys int

	// StartEnglish：開始時是英文模式（#160，settings 的 start_mode=english）。新 session 從英文開始；
	// SharedMode 時是引擎啟動（登入）後第一個共用的模式。零值從注音開始，跟以前一樣。
	StartEnglish  bool
	sessions      map[uint64]*session.Session
	next          uint64
	sharedInit    bool              // sharedEnglish 已依 StartEnglish 設好初值（宿主在 New 之後才設定選項）
	sharedEnglish bool              // SharedMode 時大家共用的模式
	modeGen       uint64            // sharedEnglish 每次改變時加一
	modeSeen      map[uint64]uint64 // session → 它跟上的 modeGen
	modeLocal     map[uint64]bool   // 「設成英文」帶來、只屬於這個輸入框的英文（macOS 的 Caps Lock）；設成中文或切換時結束

	learnedUndo []learnedUndo // 「靈犀學到的」頁最近幾次移除、不再學、恢復學習的復原複本（learned.go），只在記憶體
	undoSerial  uint64

	supportOnce sync.Once // 標音用的字讀音詞頻（vocab_annotate.go），第一次用到才建
	support     map[rune]map[zhuyin.Syllable]float64
}

func New(lex lexicon.Source, sys lm.System) *Engine {
	return &Engine{Model: personal.NewModel(lex, sys), sessions: map[uint64]*session.Session{}}
}

// IdleKeys 回報沒在組字、注音模式時輸入法會處理的可見 ASCII 按鍵（session.IdleKeys）。
// 給自己過濾閒置按鍵的外殼（Windows 的 TestKeyDown）查詢。
func (e *Engine) IdleKeys() string { return session.IdleKeys() }

// IdleKeysFullWidth 回報全形（Shift＋空白，#278）時沒在組字、注音模式會處理的可見 ASCII 按鍵（session.FullWidthIdleKeys）。
// Windows 外殼的 TestKeyDown 在快照的 full_width 為 true 時照這份決定。
func (e *Engine) IdleKeysFullWidth() string { return session.FullWidthIdleKeys() }

func (e *Engine) OpenSession() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.next++
	e.sessions[e.next] = session.New(e.Model)
	e.sessions[e.next].Rerank = e.Rerank
	e.sessions[e.next].Punct = e.Punct
	e.sessions[e.next].DocumentContextOff = e.DocumentContextOff
	e.sessions[e.next].EscKeepComposition = e.EscKeepComposition
	e.sessions[e.next].AdvanceAfterSelect = e.AdvanceAfterSelect
	e.sessions[e.next].SelectionKeys = e.SelectionKeys
	e.sessions[e.next].NoLearning = e.NoLearning
	e.sessions[e.next].InputStyle = e.InputStyle
	e.sessions[e.next].Fuzzy = e.Fuzzy
	e.sessions[e.next].EnglishAutocomplete = e.EnglishAutocomplete
	e.sessions[e.next].CandidateColumns = e.CandidateColumns
	e.sessions[e.next].CandidateBeforeCursor = e.CandidateBeforeCursor
	e.sessions[e.next].Preview02 = e.Preview02
	if e.English != nil {
		e.sessions[e.next].English = e.English
	}
	if e.DocumentContextWeight != nil {
		e.sessions[e.next].Opt.DocumentContextWeight = *e.DocumentContextWeight
	}
	e.sessions[e.next].Opt.PrefixCacheKeys = e.PrefixCacheKeys
	if e.initialEnglish() {
		e.sessions[e.next].Handle(proto.Request{Type: proto.Mode, On: true})
	}
	return e.next
}

// InitialEnglish 回報新 session 開始時的模式（#160）：SharedMode 時是目前共用的模式，否則是 StartEnglish。
// 外殼在還沒送出按鍵前據此顯示中／英、決定第一個按鍵要不要攔。
func (e *Engine) InitialEnglish() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.initialEnglish()
}

// initialEnglish 需持有 e.mu。
func (e *Engine) initialEnglish() bool {
	if !e.SharedMode {
		return e.StartEnglish
	}
	e.initShared()
	return e.sharedEnglish
}

// initShared 需持有 e.mu：共用的模式第一次用到時從開始時的模式起算。
func (e *Engine) initShared() {
	if !e.sharedInit {
		e.sharedInit, e.sharedEnglish = true, e.StartEnglish
	}
	if e.modeSeen == nil {
		e.modeSeen = map[uint64]uint64{}
		e.modeLocal = map[uint64]bool{}
	}
}

// PrunePersonalIfDue 在引擎鎖內清理個人資料中衰減到不影響排序的項目，每日至多一次
// （personal.Model.PruneIfDue）；宿主在存檔前呼叫。
func (e *Engine) PrunePersonalIfDue() (personal.PruneStats, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Model.PruneIfDue()
}

// ResetPersonal 清除全部學到的資料（設定程式的「清除學習紀錄」，#301）：個人資料回到空白、「靈犀學到的」頁的
// 復原複本一起放掉（否則復原會把清掉的詞放回來），每個輸入框也丟掉清除前打的前文、正在組字的那段送出時不學。
// 自訂詞彙不動。宿主之後要立刻存檔，磁碟上的 user.json 才會跟著清空。
func (e *Engine) ResetPersonal() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Model.ResetStore()
	clear(e.learnedUndo)
	e.learnedUndo = nil
	for _, s := range e.sessions {
		s.ForgetLearning()
	}
}

// PersonalSnapshot 在引擎鎖內複製個人資料，讓宿主在鎖外存檔，不跟提交同時讀寫 map。
func (e *Engine) PersonalSnapshot() *personal.Store {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Model.Store.Clone()
}

// Session 回傳指定的 session，供同行程的宿主（CLI、測試）調整選項。
func (e *Engine) Session(id uint64) *session.Session {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sessions[id]
}

// Sessions 回報 session 數與其中正在組字的數量（引擎交接與健康查詢用；不含文字）。
func (e *Engine) Sessions() (total, composing int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.sessions {
		total++
		if s.Composing() {
			composing++
		}
	}
	return total, composing
}

func (e *Engine) CloseSession(id uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.sessions, id)
	delete(e.modeSeen, id)
	delete(e.modeLocal, id)
}

// Do 處理一個已解碼的請求。
func (e *Engine) Do(id uint64, req proto.Request) (proto.Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.sessions[id]
	if !ok {
		return proto.Snapshot{}, fmt.Errorf("session %d 不存在", id)
	}
	if !e.SharedMode {
		return s.Handle(req), nil
	}
	// 全部共用（#115）：只有使用者的切換（Toggle）改變共用的模式。外殼的「設成中文」（Fcitx5／IBus 換輸入框時的
	// 重設、macOS 關掉 Caps Lock）改成回到共用的模式；「設成英文」（macOS 開 Caps Lock）只影響這個輸入框。
	// Windows／Linux 的 caps_lock（#173）同理：開啟只影響這個輸入框，關閉回到共用的模式。
	// 舊外殼不必改。
	e.initShared()
	// 暫時的英文（Caps Lock 開著：#173 的 caps_lock、macOS 的「設成英文」）期間不跟上別的輸入框切回的注音，
	// 不然 Caps Lock 還開著，下一個字母就打成注音；關掉 Caps Lock 時才回到共用的模式（下面與「設成中文」）。
	// 共用的模式已經是英文時的「設成英文」（Windows 重開 session 還原共用的英文）不算暫時的，照常跟上。
	temporary := !e.sharedEnglish && e.modeLocal[id]
	switch {
	case req.Type == proto.Mode && !req.On:
		req.On = e.sharedEnglish
		delete(e.modeLocal, id)
	case req.Type == proto.Mode:
		if !e.sharedEnglish {
			e.modeLocal[id] = true
		} else {
			delete(e.modeLocal, id)
		}
	case req.Type == proto.Toggle:
		delete(e.modeLocal, id)
	}
	if seen, ok := e.modeSeen[id]; (!ok || seen != e.modeGen) && s.EnglishMode() != e.sharedEnglish && req.Type != proto.Mode && !temporary {
		s.Handle(proto.Request{Type: proto.Mode, On: e.sharedEnglish}) // 跟上別的輸入框切換後的模式
	}
	snap := s.Handle(req)
	if req.Type == proto.Toggle && s.EnglishMode() != e.sharedEnglish {
		e.sharedEnglish = s.EnglishMode()
		e.modeGen++
	}
	e.modeSeen[id] = e.modeGen
	return snap, nil
}

// Handle 處理一個 JSON 編碼的請求，回傳 JSON 編碼的快照。
func (e *Engine) Handle(id uint64, req []byte) ([]byte, error) {
	var r proto.Request
	if err := json.Unmarshal(req, &r); err != nil {
		return nil, fmt.Errorf("請求格式錯誤：%w", err)
	}
	snap, err := e.Do(id, r)
	if err != nil {
		return nil, err
	}
	return json.Marshal(snap)
}
