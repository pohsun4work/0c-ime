// Package session 是單一輸入欄位的狀態機。核心擁有全部狀態：組字、游標、鎖定、
// 候選、補全與學習事件；外殼只送請求、畫快照（docs/19 原則 1）。
package session

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/fuzzy"
	"github.com/0c-biggo/0c-ime/core/input"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/punct"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/suggest"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

const perPage = 10

type candState struct {
	start, length int
	before        bool // 前方模式：詞長變動仍以原來的尾端為界，向左延伸
	items         []decode.Candidate
	cursor        int              // 反白的候選（items 的索引）；頁碼由它推得
	literal       bool             // 符號候選：插入或替換字面文字，不作為詞彙學習
	pending       bool             // 選取後消耗尚未完成的注音；取消時保留
	raw           bool             // 一音節漢字候選的最後一項是原樣注音
	matched       []string         // 與 items 等長（或 nil）：近音候選在詞庫中的讀音，原音為空字串
	labels        []string         // 開啟候選時算一次的顯示原因；不隨反白或翻頁改變
	english       bool             // 英文模式的自動補字候選
	mixed         bool             // 尚未定案的中英按鍵候選，保留原始按鍵
	keyLiterals   map[int]string   // candidate index → original physical keys for a phonetic span
	mixedChars    map[int]string   // 中英按鍵候選裡的中文字（#146）：candidate index → 字，讀音是 mixedSyl
	mixedAt       int              // mixedChars 的按鍵從待定按鍵的這個位置起，前段照字面
	mixedSyl      zhuyin.Syllable  // mixedChars 的讀音
	syllable      *pendingLiteral  // 原樣留下的音節（#109）：選項是同讀音的各聲調，選入後仍是原樣音節
	chars         int              // 原樣音節自己有字時（#296），items[chars:] 是這個讀音的字，選了換回一般的字；0 表示沒有
	flips         map[int]flipPick // ↓ 翻邊（P8）：candidate index → 換法（切法 2萬、英數組得出的 孬）
	span          *spanBack        // 同一串按鍵的清單（修改「2萬」，openFlipSpan）：Tab 回去的清單
	kind          string
	direct        bool  // empty composition: selection commits once.
	ids           []int // stable palette IDs, independent of recent order.
}

func (s *Session) candidatePage() int { return s.cand.cursor / s.pageSize() }

// hasExact 回報目前詞長是否有原音候選（非近音）。
func (c *candState) hasExact() bool {
	for i := range c.items {
		if i >= len(c.matched) || c.matched[i] == "" {
			return true
		}
	}
	return false
}

// Options 是 session 層級的行為開關。
type Options struct {
	DocumentContextWeight float64         // 經確認的跨提交前詞與 BOS 混合權重，0 停用
	PrefixAware           bool            // 組字尾端視為更長的詞的前綴
	Suggest               suggest.Options // Tab 補全的門檻
	Beam                  int             // 每個位置至多展開幾個有上下文的狀態；0 不限（見 decode.Decoder.Beam）
	Continue              float64         // 尾端「還會繼續打一個更長的詞」的權重 γ（見 decode.Decoder.Continue）
	SeparateContinuation  bool
	FuzzyAuto             fuzzy.AutoPolicy // 模糊音 auto 模式的自動修正門檻；只在有配對設為 auto 時用到
	PrefixCacheKeys       int              // 尾端前綴快取最多幾個讀音；0 為預設 4096（見 decode.Decoder.PrefixCacheKeys），只影響速度與記憶體
}

func DefaultOptions() Options {
	return Options{DocumentContextWeight: 0.1, PrefixAware: true, Suggest: suggest.DefaultOptions(), Beam: 8, Continue: 0.5, SeparateContinuation: true, FuzzyAuto: fuzzy.DefaultAutoPolicy()}
}

type Session struct {
	styleCycle *candState
	InputStyle inputstyle.Rules
	fullWidth  bool
	styleGrid  bool
	// Rerank 由宿主在第一次 Handle 前設定；零值保持原引擎。
	Rerank rerank.Options
	// Punct 由宿主設定：提交時補標點（只插入、不改原字）；nil 停用。私人／暫停／英數狀態一律不送出文字。
	Punct punct.Punctuator
	// Fuzzy 由宿主依使用者設定：逐對啟用的模糊音；零值全部關閉，行為與沒有此功能時相同。
	Fuzzy fuzzy.Config
	// Toneless 由宿主依「不打聲調也能選字」設定（#150，預設關）：聲調選填，沒打聲調的音節由解碼決定（untoned.go）。
	// 開啟時停用中英混打的自動判斷。零值時行為與沒有此功能時相同。
	Toneless bool
	// Abbreviation 由宿主設定（手機預設開）：聲調選填時只打聲母或介音也能選字（ㄋㄏ 是「你好」）。
	// 只在聲調選填生效時作用；零值時行為與沒有此功能時相同。
	Abbreviation bool
	// MixedWithToneless 由宿主設定（手機）：聲調選填與中英混打共存。聲調選填和簡拼只用在注音鍵
	// （symbol 請求，手機的注音鍵盤）；實體按鍵照混打規則判斷英文或標準注音。零值時兩者二選一（#150）。
	MixedWithToneless   bool
	symbolPending       bool                                  // 未打完的注音來自注音鍵（symbol 請求）；接著的實體按鍵要先把它完成（finishSymbolPending）
	sylPrior            map[zhuyin.Syllable]float64           // 簡拼保留讀音用的單音節常用程度（syllablePrior）
	missedMemo          map[zhuyin.Syllable][]zhuyin.Syllable // 打的音節補上一鍵的讀音（missedFor，#335）
	repairMemo          repairMemo                            // 待定按鍵顯示的修正讀音（mixedRepairShown）
	spaceMemo           spaceMemo                             // 待定按鍵接空白的決定（mixedSpaceAction，P7）
	EnglishAutocomplete bool
	// KeepRedundantKeys：「重複按的注音當成一次」關閉時由宿主設定（P6）。混打時重複的注音不合併，照官方（wuuuu/6 是 wuuu營）。
	KeepRedundantKeys bool
	English           *english.Dictionary // 共用、不會改變的英文詞庫
	// equalsPalette：= 開 Emoji／片語選單。fork 的 = 直接打 =（P4.5），選單到 P10／P11 才有新入口；目前只有測試打開它。
	equalsPalette         bool
	CandidateColumns      int
	CandidateBeforeCursor bool // 由宿主設定；零值維持游標後方選字
	displayColumns        int
	paletteCommit         bool
	paletteChanged        bool
	paletteUndo           *literalSpan
	englishWord           bool // 只有本功能開始的英文單字才在邊界送出
	englishDismissed      bool
	englishTokenBlocked   bool // 網址／識別字：到下一個空白或導覽前不重新補字
	englishSpaceCommit    bool // 英文模式這個字是按空白送出的：後面還會接著打，不是單獨打完的一串（#328）
	mixed                 pendingKeys
	mixedKeys             map[int]string      // completed phonetic position → original keys, retained until edited/committed
	mixedOrigins          map[int]mixedOrigin // 字面文字：使用者打的或系統補的；音節：選過的注音文、神經判過的（P8）
	mixedRevisits         []*mixedRevisit     // 神經比過、之後帶著後文再比一次的混打決定（P7，最多 mixedRevisitSlots 個）
	mixedSpent            time.Duration       // 這個請求裡 P7 花掉的時間（每鍵預算 mixedNeuralDeadline）
	mixedAsked            bool                // 這個請求裡已經為決定問過神經：回頭比留到下一鍵
	mixedBusy             int                 // 評分超過期限後還有幾鍵不問神經（mixedBusyKeys）
	pendingToneKey        *toneKeyPending     // 上一鍵是單獨的聲調鍵：緊接著的空白換成聲調符號（#135）
	toneKeyDone           string              // 上一鍵用這個聲調鍵打完一個字：緊接著再按同一個鍵是連按（#321）
	untonedRun            *untonedRun         // 聲調選填時這一串連續打的注音鍵（#150），認「我的詞彙」的英文詞
	literalKeys           map[int]string      // 照字面留下的符號位置 → 原按鍵，↓ 第二項可選回（#135 聲調符號、#190 混打的「，」「。」）
	retypeLiteral         bool                // 上一鍵剛把沒有字的音節原樣留下：緊接著的同類介音／韻母是在改它（#85）
	// DocumentContextOff 由宿主依使用者設定：外殼送來的前文確認一律忽略，不以前文選字。
	DocumentContextOff bool
	// AdvanceAfterSelect 由宿主依使用者設定「選字後游標移到下一個字」（#133）：從候選窗選定詞後，
	// 後方模式游標移到所選詞的結尾，前方模式往後移一個字（同新酷音 autoShiftCur），句尾時不動。
	// 零值游標留在原處。符號、Emoji 與原樣注音本來就插入在游標處、游標停在它們後面，不受影響。
	AdvanceAfterSelect bool
	// SelectionKeys 由宿主依使用者設定（#112）：候選窗每一頁第 1–10 項的選字鍵，例如 1234qweras。
	// 空字串或不合用的值為 1234567890。數字 1–0 不論設定都照位置選字。
	SelectionKeys string
	// NoLearning 由宿主依「記住我選的字詞」設定（#297，預設開；關閉時為 true）：送出、手選、選表情與符號都不寫進個人資料
	// （詞的學習、更正、英文單字、中英混打的注音文、最近用過的表情與符號、= 選單的分頁）。其他照常：神經選字、
	// 依前文選字、英文自動補字的建議，已經學到的資料也照常參與排序；「我的詞彙」的加詞、移除與不再學照常。
	// 跟暫停學習（proto.Pause）、私人欄位不同，那兩個另外停掉推論、前文與個人候選。零值照常學習。
	NoLearning bool
	// Preview02 打開時處理 0.2.0 的按鍵（組字中加詞、反悔鍵、「我的片語」，#128）；0.2.0 起宿主一律打開。
	// 零值時這些按鍵照舊回 handled=false 交還應用程式，行為與沒有此功能時相同（測試用它確認這一點）。
	Preview02    bool
	mark         *markState // 組字中加詞的標示範圍（#123）
	notices      []string   // 這次請求中由按鍵處理產生的提示，併入快照
	vocabChanged bool
	forgot       bool
	// learningCleared：組字途中清除了學習紀錄（ForgetLearning，#301）：這段組字的手選與內容是清除前打的，
	// 送出時不學；送出或取消後（reset）恢復。
	learningCleared bool
	// EscKeepComposition 由宿主依使用者設定：Esc 只清除未打完的注音，保留已組好的字；零值清除整段（多數注音輸入法的預設）。
	EscKeepComposition bool
	allowRerank        bool
	documentEnabled    bool
	documentLeft       string
	pendingLeft        string
	predictions        []string // 剛列出的聯想詞；只在下一個請求之前有效（predict.go）
	predictedLeft      string   // predictions 接在哪個前詞之後；開頭詞是句首
	neuralLeft         string   // 外殼確認後的自身提交文字（最多 rerank.MaxContextRunes 字），只給宣告需要前文的評分器
	pendingNeural      string
	rerankCache        map[string]rerankEntry // 分段神經決策快取；提交／取消時清空
	rerankMemo         map[int]segMemo        // 段級輸入摘要：輸入全同的段整段跳過（含 N-best）；提交／取消時清空
	rerankMemoHits     int                    // rerankMemo 命中次數，只給測試確認打字時真的命中
	noCaches           bool                   // 測試用：不用 rerankMemo 與 sugPrev，與開著時比對結果（rerank_caches_test.go）
	sugPrev            *suggest.Suggestion    // 補全建議與其輸入摘要；refresh 每鍵跑好幾次，同輸入不重算
	sugPrevKey         string
	phonetic           *phoneticHint
	phoneticCache      phoneticHintCache
	phoneticUndo       *phoneticUndo
	hint               *neuralHint       // 最後一段的次佳候選提示；Tab 接受
	shownPath          []decode.Seg      // 上次顯示的路徑（含神經改選）；下一鍵重新評分時延伸成候選
	shownSyls          []zhuyin.Syllable // 產生 shownPath 的讀音
	pendingContextID   uint64
	model              *personal.Model
	// Spatial 由宿主設定：觸控鍵盤的相鄰鍵容錯（spatial.go）。零值關閉。
	Spatial SpatialOptions
	// compNear 是組音中各類注音的鄰鍵；spatial 是已完成音節（依位置）換成鄰鍵的讀音；pendingSpatial 交給下一次 insert。
	compNear       map[zhuyin.Class]spatialSlot
	spatial        map[int][]input.Hyp
	pendingSpatial []input.Hyp
	pendingNear    *spatialSlot // 這次 symbol 呼叫帶的鄰鍵（symbolNear 設定）
	// lastLearn 是上一次提交（或點聯想詞）學習的復原複本；UndoLearn 用它還原一次（觸控：送出後立刻退格）。
	lastLearn *personal.Undo
	dec       *decode.Decoder
	front     input.Frontend
	Opt       Options

	comp            zhuyin.Composer
	pendingLiterals []pendingLiteral // 暫存成字面注音的組音狀態，撤銷旁邊的英數後可接續
	toggleStash     *pendingStash    // 切到英文時清掉的未打完注音；下一個請求不是立即切回就丟棄（#90）
	syls            []zhuyin.Syllable
	cursor          int
	locks           map[int]decode.Lock
	literals        map[int]bool   // syls 中此位置是字面 Unicode scalar；編輯依 grapheme 邊界
	displayed       map[int]string // 已顯示的漢字；英數切開詞條而解不出時保留畫面
	nearShown       map[int]string // 近音保底時避開沒有字的音節、其餘各段照常解出的字（decodeAroundUnreadable）
	rejected        []personal.RejectedSeg
	paused          bool
	private         bool         // 敏感欄位組字持續到清空，切回普通欄位也不把殘留文字學進去
	localBlocked    bool         // 暫停／敏感狀態進入後，本次組字不恢復暫時證據
	english         bool         // 英文模式：有組字時英數留在組字區，空白時交還應用程式
	autoLog         []AutoChange // 目前顯示路徑中的自動近音修正（診斷用，每次重新解碼時重建）
	nearReading     bool         // 精確讀音解不出路徑，目前只因近音可成詞而收進組字；顯示保底（已顯示的字照舊，其餘原樣注音）

	path     []decode.Seg
	basePath []decode.Seg // 不含暫時證據的路徑，鄰詞證據不得從自己的推測反覆累加
	pathSig  string       // 產生 path 的輸入；輸入沒變就不重新解碼
	pathGen  int          // 產生 path 時的個人模型世代（model.Gen）；Rescore 用它確認重解不會帶進之後的學習
	sug      *suggest.Suggestion
	cand     *candState
	rev      int
}

func New(m *personal.Model) *Session {
	return &Session{
		model:   m,
		dec:     &decode.Decoder{Lex: m.Source(), Score: m},
		front:   input.Exact{},
		Opt:     DefaultOptions(),
		English: english.Prototype(),
		locks:   map[int]decode.Lock{},
	}
}

// mixedInput：中英混打（fork 固定開）；手機的聲調選填預設跟混打二選一（#150）。
func (s *Session) mixedInput() bool { return !s.toneless() || s.MixedWithToneless }

func (s *Session) composing() bool {
	return len(s.syls) > 0 || !s.comp.Empty() || s.mixed.raw != ""
}

// Composing 回報是否有未提交的組字；引擎交接只在沒有組字時進行。
func (s *Session) Composing() bool { return s.composing() || s.cand != nil }

// ForgetLearning 在個人資料整份清除後（#301）丟掉這個輸入框留著、清除前打的字：前文（上一次送出的末詞，
// 下一次送出會跟它學成相鄰詞）。正在組字時畫面照舊，但這段組字送出時不學。
func (s *Session) ForgetLearning() {
	s.clearDocumentContext()
	if s.Composing() {
		s.learningCleared = true
	}
}

// Handle 處理一個請求並回傳完整快照。
func (s *Session) Handle(r proto.Request) proto.Snapshot {
	cycleKey := r.Type == proto.Key && r.Key == proto.KeyUp
	if !cycleKey && r.Type != proto.Context && r.Type != proto.Display {
		s.styleCycle = nil
	}
	s.paletteChanged = false
	// 前文確認與欄數不是使用者操作；其他任何請求都表示切到英文後已有動作，不再還原注音。
	stash := s.toggleStash
	if r.Type != proto.Context && r.Type != proto.Display {
		s.toggleStash = nil
	}
	if r.Type != proto.Context && r.Type != proto.Display && r.Type != proto.Backspace && !(r.Type == proto.Key && (r.Key == proto.KeyBackspace || r.Key == proto.KeyEscape)) {
		s.phoneticUndo = nil
	}
	if r.Type != proto.Backspace && !(r.Type == proto.Key && r.Key == proto.KeyBackspace) {
		s.paletteUndo = nil
	}
	if r.Type != proto.Key && r.Type != proto.Context && r.Type != proto.Display {
		s.pendingToneKey = nil // 按鍵由 key 自己取出；其他操作之後的空白不再換成聲調符號（#135）
		s.toneKeyDone = ""
		s.retypeLiteral = false
		s.untonedRun = nil
	}
	_, keypad := proto.KeypadChar(r.Key)
	_, ctrlPunct := proto.CtrlPunct(r.Key) // 全形標點也是打字：不重排的話插入標點後整段退回統計選字，打完 。 就 Enter 送出的是統計的結果
	s.mixedSpent, s.mixedAsked = 0, false
	s.allowRerank = r.Type == proto.Symbol || r.Type == proto.Backspace || r.Type == proto.Key && (len([]rune(r.Key)) == 1 || keypad || ctrlPunct || r.Key == proto.KeyBackspace)
	defer func() { s.allowRerank = false }()
	if s.allowRerank && s.mixedBusy > 0 {
		s.mixedBusy--
	}
	if !s.composing() {
		s.private = r.Private
	} else {
		s.private = s.private || r.Private
	}
	if r.Private || r.Type == proto.Pause && r.On {
		s.clearDocumentContext()
		s.localBlocked = true
		if s.cand != nil && (s.cand.mixed || s.cand.kind != "") {
			s.cand = nil // 不讓已開啟的個人英文候選、最近用過的表情跨入保護狀態。
		}
	}
	s.model.SyncTime()
	if r.Type != proto.Key {
		s.mark = nil // 加詞的標示只跟著按鍵；滑鼠移動游標、切換模式等都取消
	}
	var commit string
	var notices []string
	handled := true
	predicted, predictedLeft := s.predictions, s.predictedLeft // 聯想詞只對緊接的選取有效
	s.predictions, s.predictedLeft = nil, ""
	switch r.Type {
	case proto.Context:
		s.confirmDocumentContext(r)
	case proto.Display:
		if r.Columns >= 1 && r.Columns <= 10 {
			s.displayColumns = r.Columns
		}
	case proto.Key:
		if editsPending(r.Key) {
			// 編輯鍵作用在未打完的注音上：Backspace 刪一個、Esc 清掉，不先收成音節再整個刪掉。
			handled, commit = s.key(r.Key)
			s.symbolPending = s.symbolPending && !s.comp.Empty()
			break
		}
		s.finishSymbolPending()
		handled, commit = s.key(r.Key)
		s.revisitMixed()
	case proto.Symbol:
		s.settleTokenAn()
		s.settleMixed(true) // 顯示成注音的待定按鍵交給注音接著組（nj 再點 ㄅ 是 ㄅㄨ）
		for _, sym := range r.Symbol {
			handled = s.symbolNear(sym, r.Near) && handled
		}
		s.symbolPending = !s.comp.Empty()
	case proto.Backspace:
		s.englishDismissed = false
		s.cand = nil
		handled = s.backspace()
	case proto.Cursor:
		s.settleShown()
		s.moveTo(r.To)
	case proto.Candidates:
		s.openCandidates(r.Length)
	case proto.Page:
		if s.cand != nil && r.Index >= 0 && r.Index < s.pages() {
			s.cand.cursor = r.Index * s.pageSize()
		}
	case proto.Select:
		if predicted != nil && s.cand == nil && !s.composing() {
			if c, ok := s.commitPrediction(predicted, predictedLeft, r.Index); ok {
				commit = c
			} else {
				notices = append(notices, "沒有這個候選")
			}
		} else if !s.selectCandidate(r.Index) {
			notices = append(notices, "沒有這個候選")
		}
	case proto.Predict:
		s.predictions, s.predictedLeft = s.predict()
	case proto.Fix:
		if !s.fixWord(r.To, r.Length, r.Word) {
			notices = append(notices, "沒有這個候選")
		}
	case proto.Close:
		if s.englishWord {
			s.englishDismissed = true
		}
		s.cand = nil
	case proto.Accept:
		handled = s.accept()
	case proto.Complete:
		handled = s.acceptCompletion(r.Word)
	case proto.Commit:
		if r.Word != "" {
			commit, handled = s.commitShown(r.Word) // 觸控：送出畫面上的這句
		} else {
			commit = s.commitComposed()
		}
	case proto.Cancel:
		s.reset()
	case proto.Unlearn:
		s.model.Unlearn(r.Word)
		s.lastLearn = nil // 復原複本早於這次移除：還原會讓移除的證據回來
		s.pathSig = ""
	case proto.UndoLearn:
		handled = s.model.Restore(s.lastLearn)
		s.lastLearn = nil
	case proto.Forget:
		s.model.Forget(r.Word)
		s.lastLearn = nil
		if s.cand != nil && s.cand.english {
			s.cand = nil
		} else if s.cand != nil && s.cand.mixed {
			s.openMixedCandidates()
		}
		s.pathSig = "" // 明確撤銷仍立即生效，不受神經畫面保留規則影響。
		notices = append(notices, "已撤銷與「"+r.Word+"」有關的學習")
	case proto.Pause:
		s.paused = r.On
		if r.On && s.englishWord {
			commit = s.commit()
		}
	case proto.Toggle, proto.Mode:
		english := !s.english
		if r.Type == proto.Mode {
			english = r.On
		}
		s.setMode(english, r.Type == proto.Toggle && r.Restore, stash)
	default:
		handled = false
	}
	if r.Type == proto.Toggle || r.Type == proto.Mode || !handled && r.Type == proto.Key {
		s.clearDocumentContext()
	}
	if s.paletteCommit {
		s.paletteCommit = false
		s.refresh()
		commit = s.snapshot().Text() // direct palette output is exact; no punctuation/model rewrite.
		s.reset()
	}
	s.refresh()
	// 保護狀態／前文確認／撤銷可以在候選窗仍開啟時變更；不能留下過期的個人或關聯標示。
	if s.cand != nil && !s.cand.mixed && (r.Private || r.Type == proto.Pause || r.Type == proto.Context || r.Type == proto.Forget) {
		s.cand.labels = s.candidateLabels(s.cand)
	}
	s.rev++
	s.refreshEnglishCandidates()
	snap := s.snapshot()
	if hint := s.markHint(); hint != "" {
		s.notices = append(s.notices, hint) // 標示中每個快照都附說明（#123；三平台調查都沒有，最容易拉開差距）
	}
	snap.Handled, snap.Commit, snap.Notices = handled, commit, append(notices, s.notices...)
	snap.PersonalChanged = s.paletteChanged || s.forgot
	snap.VocabChanged, snap.Forgot = s.vocabChanged, s.forgot
	s.notices, s.vocabChanged, s.forgot = nil, false, false
	if commit != "" {
		snap.ContextID = s.pendingContextID
	}
	if s.predictions != nil {
		snap.Candidates = predictionList(s.predictions, s.predictedLeft)
	}
	s.status(&snap)
	return snap
}

// View 回傳目前的畫面，不改任何狀態（不算一次操作、Revision 不變）。
func (s *Session) View() proto.Snapshot {
	snap := s.snapshot()
	snap.Handled = true
	if hint := s.markHint(); hint != "" {
		snap.Notices = []string{hint}
	}
	s.status(&snap)
	return snap
}

// status 填入快照裡不屬於組字的狀態，Handle 與 View 共用。
func (s *Session) status(snap *proto.Snapshot) {
	snap.Mode = proto.ModeZhuyin
	if s.english {
		snap.Mode = proto.ModeEnglish
	}
}

// setMode 切換中英模式（單按 Shift、外殼設定模式）。切到英文清掉未打完的注音（#90），已組好的字與游標保留；
// 切換本身也關閉候選。restore 是外殼確認中間沒有任何操作就立即切回注音：把剛清掉的注音放回原處（防誤觸）。
func (s *Session) setMode(english, restore bool, stash *pendingStash) {
	if english && !s.english {
		s.finishUntoned() // 聲調選填：已是完整音節的注音留在組字區，不當成打到一半清掉（#150）
		s.toggleStash = s.clearPendingZhuyin()
	}
	s.flushMixed()
	s.clearDocumentContext()
	s.cand = nil
	if !english && s.english && restore && stash != nil {
		s.comp, s.mixed = stash.comp, stash.mixed
	}
	s.english = english
	s.englishWord, s.englishDismissed, s.englishTokenBlocked = false, false, false
}

// EnglishMode 回報目前是否為英文模式（引擎在「全部共用」時用來同步各輸入框，#115）。
func (s *Session) EnglishMode() bool { return s.english }

func (s *Session) key(k string) (handled bool, commit string) {
	if s.paletteUndo != nil && k != proto.KeyBackspace {
		s.paletteUndo = nil
	}
	toneKey := s.pendingToneKey // 只對緊接著的這一鍵有效（#135）
	s.pendingToneKey = nil
	toneDone := s.toneKeyDone // 同上（#321）
	s.toneKeyDone = ""
	retype := s.retypeLiteral // 同上，只有緊接著的這一鍵能改剛留下的錯音節（#85）
	s.retypeLiteral = false
	run := s.untonedRun // 聲調選填時連續打的注音鍵（#150）：只有緊接著的這一鍵能接續
	s.untonedRun = nil
	if s.hoshiSymbolKey(k) {
		return true, ""
	}
	if handled, commit, done := s.styleKey(k); done {
		return handled, commit
	}
	k = s.normalizeVocabKey(k) // #123 的新鍵名在沒有對應功能時就是原本的鍵
	if s.englishShiftDelete(k) {
		return true, ""
	}
	// 數字鍵盤永遠不是注音鍵（#75）：英文模式與候選選號同主鍵盤數字；沒在組字交還應用程式；
	// 組字中以半形插在游標處，未打完的注音保留成符號（同 Shift＋字母）。
	if c, ok := proto.KeypadChar(k); ok {
		switch {
		case s.english || s.cand != nil && c >= '0' && c <= '9':
			k = string(c)
		case s.cand != nil:
			return true, "" // 候選開啟時吃掉運算鍵，不與選字同時解讀
		case !s.composing():
			return false, ""
		default:
			s.flushMixed()
			s.insertPendingLiteral()
			s.insertLiteral(string(c))
			return true, ""
		}
	}
	if p, ok := proto.CtrlPunct(k); ok {
		return s.ctrlPunct(p), ""
	}
	if s.english && s.EnglishAutocomplete {
		if handled, commit, active := s.englishKey(k); active {
			return handled, commit
		}
	}
	if s.english && !s.composing() {
		return false, ""
	}
	// 英文模式的空白、標點與數字都按原樣插入，不開候選或解讀成注音。
	if s.english {
		if runes := []rune(k); len(runes) == 1 && unicode.IsPrint(runes[0]) {
			s.insertPendingLiteral()
			s.insertLiteral(k)
			return true, ""
		}
	}
	if s.Preview02 && s.cand == nil {
		if handled, consumed := s.vocabKey(k); consumed {
			return handled, ""
		}
	}
	if s.cand != nil {
		if s.paletteKey(k) {
			return true, ""
		}
		switch k {
		case proto.KeyShiftDelete, proto.KeyShiftBackspace:
			s.forgetCandidate() // normalizeVocabKey 已確認開關打開、是一般候選
		case proto.KeyEscape, proto.KeyBackspace, proto.KeyDelete:
			s.cand = nil // 關閉候選、保留組字；Backspace／Del 不刪字（#106）
		case proto.KeyEnter:
			s.selectCandidate(s.candidateID(s.cand.cursor))
			if s.InputStyle.CandidateEnter == "commit" {
				return true, s.commit()
			}
		case " ":
			if s.InputStyle.CandidateSpace == "select" {
				s.selectCandidate(s.candidateID(s.cand.cursor))
			} else {
				s.cand.cursor = (s.candidatePage() + 1) % s.pages() * s.pageSize()
			}
		case proto.KeyPageDown:
			s.cand.cursor = (s.candidatePage() + 1) % s.pages() * s.pageSize()
		case proto.KeyPageUp:
			s.cand.cursor = (s.candidatePage() + s.pages() - 1) % s.pages() * s.pageSize()
		case proto.KeyDown:
			if s.candidateColumns() > 1 {
				s.gridMove(k)
			} else {
				s.moveHighlight(1)
			}
		case proto.KeyUp:
			if s.candidateColumns() > 1 {
				s.gridMove(k)
			} else {
				s.moveHighlight(-1)
			}
		case proto.KeyRight, proto.KeyLeft:
			if s.candidateColumns() > 1 {
				s.gridMove(k)
			} else if k == proto.KeyRight {
				s.cand.cursor = (s.candidatePage() + 1) % s.pages() * s.pageSize()
			} else {
				s.cand.cursor = (s.candidatePage() + s.pages() - 1) % s.pages() * s.pageSize()
			}
		case proto.KeyHome:
			s.cand.cursor = 0 // 第一個候選（#106）
		case proto.KeyEnd:
			s.cand.cursor = len(s.cand.items) - 1 // 清單的最後一個候選
		case proto.KeyTab:
			switch s.InputStyle.CandidateTab {
			case "grid":
				s.styleGrid = true
			case "next":
				s.moveHighlight(1)
			default:
				s.cycleCandidateLength(1)
			}
		case proto.KeyBackTab: // Tab 的反方向
			switch s.InputStyle.CandidateTab {
			case "next":
				s.moveHighlight(-1)
			case "length":
				s.cycleCandidateLength(-1)
			}
		default:
			if n := s.selectionIndex(k); n >= 0 {
				s.selectCandidate(s.candidateID(s.candidatePage()*s.pageSize() + n))
			} else if k == "j" { // 選字鍵優先：asdfghjkl; 時上面已選字（#193）
				s.stepCandidate(-1)
			} else if k == "k" {
				s.stepCandidate(1)
			}
		}
		return true, "" // 候選開啟時吃掉所有按鍵（含 Shift+Tab 等沒有作用的鍵），不與注音鍵同時解讀、不交給應用程式
	}
	if k == "=" && s.equalsPalette && !s.private && s.paletteTriggerAllowed() {
		s.openPalette()
		return true, ""
	}
	if s.mixedKey(k) {
		return true, ""
	}
	if s.untonedEnglish(run, k) {
		return true, "" // 「我的詞彙」裡的英文詞接空白照字面送出（#150，同混打的 #146）
	}
	if s.untonedSeparator(k) {
		return true, ""
	}
	s.untonedKey(k)
	switch k {
	case proto.KeyBackspace:
		return s.backspace(), ""
	case proto.KeyDelete:
		if !s.composing() {
			return false, ""
		}
		s.settleShown()
		s.insertPendingLiteral()
		at := s.cursor
		s.cursor = s.literalStep(at, 1)
		for s.cursor > at {
			s.eraseBeforeCursor()
		}
		return true, ""
	case proto.KeyEnter:
		if !s.composing() {
			return false, ""
		}
		return true, s.commit()
	case proto.KeyEscape:
		if s.undoPhonetic() {
			return true, ""
		}
		if s.mixed.accepted != "" {
			s.mixed.accepted = ""
			return true, ""
		}
		// 未打完的注音先單獨清掉，已組好的字留著；再按一次才依設定清除整段。
		if s.mixed.raw != "" {
			s.clearMixed()
			return true, ""
		}
		if !s.comp.Empty() {
			s.comp.Reset()
			return true, ""
		}
		if !s.composing() {
			return false, ""
		}
		if !s.EscKeepComposition {
			s.reset()
		}
		return true, "" // 保留組字時仍吃掉 Esc，不讓應用程式在組字中關閉對話框
	case proto.KeyTab:
		// 補全照舊；組字中沒有可接受的補全時吃掉不動作，不讓應用程式在組字中移動焦點或插入定位字元（#106）。
		if s.mixed.raw != "" {
			return s.acceptMixed() || s.composing(), ""
		}
		if s.english {
			return s.composing(), ""
		}
		if s.phonetic != nil || s.sug != nil {
			return s.accept() || s.composing(), ""
		}
		if s.hint != nil && s.cursor == len(s.syls) && s.comp.Empty() && s.cand == nil {
			return s.acceptHint() || s.composing(), ""
		}
		return s.composing(), "" // 沒有小視窗：組字中吃掉，沒有組字才交還應用程式
	case proto.KeyUp, proto.KeyPageUp, proto.KeyPageDown, proto.KeyBackTab:
		// 候選沒開時這些鍵在組字中沒有作用；吃掉不動作，免得應用程式的游標或焦點離開組字區（#106）。
		// 沒有組字時交還應用程式。
		return s.composing(), ""
	case proto.KeyLeft, proto.KeyRight, proto.KeyHome, proto.KeyEnd:
		if !s.composing() {
			return false, ""
		}
		if k == proto.KeyRight && s.mixedRight() {
			return true, ""
		}
		s.settleShown()
		if s.english {
			// 英文模式按畫面字元移動，不能被尚未完成的注音卡住。
			s.insertPendingLiteral()
		}
		// Home／End 移到組字區頭尾（#106）。注音打到一半時同左右鍵：游標不動、按鍵照樣吃掉，
		// 不讓應用程式的游標離開組字區。
		switch k {
		case proto.KeyLeft:
			s.moveTo(s.literalStep(s.cursor, -1))
		case proto.KeyRight:
			s.moveTo(s.literalStep(s.cursor, 1))
		case proto.KeyHome:
			s.moveTo(0)
		default:
			s.moveTo(len(s.syls))
		}
		return true, ""
	case proto.KeyDown:
		if s.english {
			return false, ""
		}
		if !s.composing() {
			return false, ""
		}
		s.openCandidates(0)
		return true, ""
	}
	runes := []rune(k)
	if len(runes) != 1 {
		return false, ""
	}
	if runes[0] == ' ' {
		if s.finishToneKey(toneKey) {
			return true, "" // 單獨的聲調鍵＋空白：聲調符號（#135）
		}
		s.restorePendingLiteral() // 返回暫存注音後，空白仍須能完成一聲。
		if s.comp.Empty() {
			if len(s.syls) == 0 {
				return false, ""
			}
			// 中英混打：注音打完後空白就是半形空白（同英文單字後的空白），↓ 選字（#78）。
			// 緊接在數字後面（連打的聲調鍵 34、數字鍵盤）也是空白，同沒在組字時打數字再按空白（#135）。
			if s.mixedInput() || s.literalNumberBefore() != "" {
				sp := " "
				if s.fullWidth {
					sp = s.fullWidthText(sp) // 全形時照微軟是全形空白
				}
				s.insertLiteral(sp)
				return true, ""
			}
			s.openCandidates(0)
			return true, ""
		}
	}
	if sym, ok := layout.Standard.Symbol(runes[0]); ok {
		if retype && s.retypeSyllableLiteral(runes[0]) {
			return true, ""
		}
		before := s.untonedRunFrom(run)
		hadReading := !s.comp.Empty()
		if s.pressKey(runes[0]) {
			s.trackUntonedKey(before, k)
			if hadReading && s.comp.Empty() {
				s.toneKeyDone = k // 這一鍵剛打完一個字（#321）；toneKey 只在它是單獨的聲調鍵時才看
			}
			return true, ""
		}
		// 沒有待完成音節的聲調鍵不可漏到應用程式，否則數字會跑到組字之前。
		// 沒開中英混打時，沒在組字也先留在組字區，緊接著空白就是聲調符號（#135）。
		if s.toneKey(k, sym, toneDone == k) {
			return true, ""
		}
	}
	// 標點保留在組字區，可移動、替換候選；Enter 才提交。
	if _, ok := layout.Standard.Punct(runes[0]); ok {
		s.insertPendingLiteral()
		s.insertPunctKey(k)
		return true, ""
	}
	// 其他可見字元（Shift＋字母、@、#、落單聲調鍵等）在游標處逐字保留；明確打的符號（@ #）沒在組字時也一樣。
	if (s.composing() || s.explicitSymbol(k)) && unicode.IsPrint(runes[0]) {
		s.insertPendingLiteral()
		s.insertLiteral(k)
		return true, ""
	}
	return false, ""
}

// ctrlPunct 處理 Ctrl＋標點（#168）：英文模式時不處理，外殼照一般快捷鍵先送出組字再交給應用程式。
// 中文模式跟一般標點（Shift＋, 等）走同一條路：未打完的注音留成符號，標點插在游標處、留在組字區，沒在組字時也一樣。
// 中英混打打到一半的英文先定案（Shift＋, 在原樣英文後是半形 <，Ctrl＋, 一律是全形）。候選開啟時同其他按鍵吃掉不動作。
func (s *Session) ctrlPunct(p string) bool {
	if s.english {
		return false
	}
	if s.cand != nil {
		return true
	}
	s.flushMixed()
	s.insertPendingLiteral()
	s.insertLiteral(p)
	return true
}

// pressKey 照標準排列解讀注音鍵：一般符號加進暫存；聲調鍵（含空白的一聲）照 symbol 完成音節，暫存是空的時候回傳 false（落單的聲調鍵，#135）。
func (s *Session) pressKey(key rune) bool {
	s.restorePendingLiteral()
	next, sym, ok := layout.Standard.Press(s.comp, key)
	if !ok {
		return false
	}
	if zhuyin.ClassOf(sym) != zhuyin.Tone && !s.MixedWithToneless && s.untonedBreak(sym) {
		s.finishUntoned() // 聲調選填：這個注音接不上目前的音節，目前的音節不打聲調就完成，這一鍵開始新的音節（#150）
		next, sym, _ = layout.Standard.Press(s.comp, key)
	}
	if zhuyin.ClassOf(sym) == zhuyin.Tone {
		if s.comp.Empty() {
			return false
		}
		return s.symbol(sym)
	}
	s.comp = next
	return true
}

// symbolNear 是帶鄰鍵的注音鍵（觸控）：放不進組音時改用放得進的鄰鍵，並記下鄰鍵供音節格使用。
func (s *Session) symbolNear(sym rune, near []proto.NearKey) bool {
	if !s.Spatial.on() {
		return s.symbol(sym)
	}
	keys := s.nearKeys(near)
	// 沒有鄰鍵的鍵也要看：接不上時可能是前面那一鍵按錯（ㄒ＋ㄚ）。
	if alt, rest, ok := s.spatialSubstitute(sym, keys); ok {
		sym, keys = alt, rest
	}
	s.pendingNear = &spatialSlot{sym, keys}
	defer func() { s.pendingNear = nil }()
	return s.symbol(sym)
}

func (s *Session) symbol(sym rune) bool {
	s.restorePendingLiteral()
	if s.untonedBreak(sym) {
		s.finishUntoned() // 聲調選填：這個注音接不上目前的音節，目前的音節不打聲調就完成（#150）
	}
	before := s.comp
	if s.comp.Empty() {
		s.compNear = nil
	}
	if p := s.pendingNear; p != nil && p.sym == sym {
		s.noteSpatial(sym, p.near)
	} else if s.compNear != nil {
		delete(s.compNear, zhuyin.ClassOf(sym))
	}
	done, ok := s.comp.Add(sym)
	if ok {
		s.completeSyllable(before, s.abbreviatedTone(before, done))
		return true
	}
	return zhuyin.ClassOf(sym) != zhuyin.None && !(zhuyin.ClassOf(sym) == zhuyin.Tone && s.comp.Empty())
}

// completeSyllable 收進一個剛完成的音節：before 是完成前的組音狀態，done 是完成的音節（打了聲調，
// 或聲調選填時沒打聲調的音節）。
func (s *Session) completeSyllable(before zhuyin.Composer, done zhuyin.Syllable) {
	done = s.neutralFallback(done)
	if s.joinPrefixLiteral(done) {
		return
	}
	// 收進這個音節之後若整段解不出任何路徑（例如 ㄏㄟˊ、單打的 ㄅ＋空白，或候選關閉後多按了
	// 一個數字鍵組出的 ㄅㄨㄛˇ），收進去會讓組字區空白，之後打什麼都顯示不出來（使用者回報）。
	// 改成把整個音節連同聲調原樣留在組字區（#109、#107）：看得到、可退格，Enter 照樣送出；
	// 使用者明確打的聲調不丟，接著打的注音從新的音節開始，不會蓋掉它。
	// 原本就解不出路徑（保底顯示中）時不擋，照舊收進去，否則會變成什麼都打不進去。
	s.refresh()
	decodable := len(s.syls) == 0 || len(s.path) > 0 || s.nearReading
	saved := s.saveSyllables()
	s.pendingSpatial = s.spatialAlternatives(before, done)
	s.compNear = nil
	s.insert(done)
	// 精確讀音沒有字、但展開近音後整段可成詞的音節照樣收進來（例如 ㄕㄨㄥˋ 打的是 ㄙㄨㄥˋ）：
	// 顯示維持原樣注音，只有開候選時才出現近音。
	if s.refresh(); decodable && len(s.path) == 0 {
		if s.fuzzyDecodable() {
			s.nearReading = true
		} else {
			s.restoreSyllables(saved)
			s.insertSyllableLiteral(before, done)
			s.retypeLiteral = true // 多半是打錯：緊接著的同類介音／韻母改這個讀音（#85）
		}
	} else if decodable && len(s.path) > 0 && !s.completeDecodable() {
		// 只能當某個更長的詞的開頭、自己沒有字的音節（ㄟ 只有「工口」的 ㄟ ㄌㄨㄛˊ，#107）：
		// 解碼器會先顯示那個詞的第一個字，接著打的音節接不上時就卡成「工ㄋㄧˇ」。
		// 同樣原樣留下注音；下一個音節能和它成詞（ㄋㄧ＋ㄗ˙＝妮子）時再收回去解碼。
		s.restoreSyllables(saved)
		s.insertSyllableLiteral(before, done)
		s.pendingLiterals[len(s.pendingLiterals)-1].prefix = true
	}
}

// insert 在游標處插入一個音節；其後的鎖定右移，跨越游標的鎖定解除。
func (s *Session) insert(syl zhuyin.Syllable) {
	at := s.cursor
	s.syls = append(s.syls[:at:at], append([]zhuyin.Syllable{syl}, s.syls[at:]...)...)
	s.shiftLocks(at, +1)
	if alts := s.pendingSpatial; len(alts) > 0 {
		s.spatial = maps.Clone(s.spatial) // 存起來準備還原的那份不跟著改
		if s.spatial == nil {
			s.spatial = map[int][]input.Hyp{}
		}
		s.spatial[at] = alts
	}
	s.pendingSpatial = nil
	s.cursor++
}

func (s *Session) backspace() bool {
	if s.undoPhonetic() {
		return true
	}
	if span := s.paletteUndo; span != nil {
		s.paletteUndo = nil
		if s.cursor == span.end {
			for s.cursor > span.start {
				s.eraseBeforeCursor()
			}
			s.restorePending(true)
			return true
		}
	}
	asTyped := s.restorePending(true)
	if s.mixed.raw != "" {
		if s.mixed.accepted != "" {
			s.trimAccepted()
		} else if asTyped || !s.eraseRepaired() {
			at := s.mixedEraseAt(asTyped)
			s.mixed.raw = s.mixed.raw[:at] + s.mixed.raw[at+1:]
			s.mixed.assistedPrefix = min(s.mixed.assistedPrefix, at)
			s.mixed.edited = s.mixed.choice == 1
		}
		if s.mixed.raw == "" {
			s.clearMixed()
		}
		return true
	}
	if !s.comp.Empty() {
		s.comp = layout.Standard.Backspace(s.comp)
		return true
	}
	if s.cursor == 0 {
		return s.composing()
	}
	to := s.literalStep(s.cursor, -1)
	for s.cursor > to {
		s.eraseBeforeCursor()
	}
	s.restorePending(true)
	return true
}

// 內部替換／還原只刪指定的一格，不能觸發使用者退格的注音還原。
func (s *Session) eraseBeforeCursor() {
	at := s.cursor - 1
	s.syls = append(s.syls[:at:at], s.syls[at+1:]...)
	s.shiftLocks(at, -1)
	s.cursor--
}

// shiftLocks 處理在 at 插入（delta=+1）或刪除（delta=-1）一個音節後的鎖定與拒絕紀錄。
// 完全在編輯點之前的鎖定不動；完全在之後的平移；被這次編輯切到的解除。
func (s *Session) shiftLocks(at, delta int) {
	s.shiftMixedKeys(at, delta)
	s.literalKeys = shiftKeys(s.literalKeys, at, delta)
	s.spatial = shiftKeys(s.spatial, at, delta)
	s.shiftMixedOrigins(at, delta)
	s.shiftLiterals(at, delta)
	s.shiftPendingLiterals(at, delta)
	s.shiftDisplayed(at, delta)
	next := map[int]decode.Lock{}
	for start, l := range s.locks {
		after := start >= at
		if delta < 0 {
			after = start > at
		}
		switch {
		case start+l.Len <= at:
			next[start] = l
		case after:
			next[start+delta] = l
		}
	}
	s.locks = next
	kept := s.rejected[:0]
	for _, r := range s.rejected {
		if r.End <= at {
			kept = append(kept, r)
		}
	}
	s.rejected = kept
}

func (s *Session) moveTo(to int) {
	s.finishUntoned()
	if !s.comp.Empty() {
		return // 音節打到一半不移動游標
	}
	s.cursor = s.literalFloor(max(0, min(to, len(s.syls))))
	s.cand = nil
}

func (s *Session) lattice() input.Lattice { return s.lockLattice(s.expand(s.syls), 0, 0) }

// signature 描述決定解碼結果的所有輸入。只打了聲母、介音等未完成符號時音節序列不變，
// 不需要重新解碼。
func (s *Session) signature() string {
	var b strings.Builder
	cursor := s.cursor
	generation := s.model.Gen
	if s.Rerank.Scorer != nil {
		cursor = 0
		generation = 0
	} // 導覽或其他欄位的學習不換掉已顯示文字；下次修改音節才採用最新模型。
	b.WriteString(zhuyin.Key(s.syls))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(cursor))
	b.WriteByte('|')
	b.WriteString(strconv.FormatBool(s.Opt.PrefixAware))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(generation))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(s.Opt.Beam))
	b.WriteByte('|')
	b.WriteString(strconv.FormatFloat(s.Opt.Continue, 'g', -1, 64))
	b.WriteByte('|')
	b.WriteString(strconv.FormatBool(s.paused))
	b.WriteByte('|')
	b.WriteString(strconv.FormatBool(s.private))
	b.WriteByte('|')
	b.WriteString(strconv.FormatBool(s.Opt.SeparateContinuation))
	b.WriteByte('|')
	b.WriteString(strconv.Quote(s.contextLeft(0)))
	b.WriteByte('|')
	b.WriteString(strconv.Quote(s.neuralContext()))
	b.WriteByte('|')
	b.WriteString(strconv.FormatFloat(s.Opt.DocumentContextWeight, 'g', -1, 64))
	b.WriteByte('|')
	if s.Fuzzy.AutoEnabled() { // 自動修正的規則、模式與門檻都決定顯示路徑；關閉時簽章與沒有此功能時相同
		b.WriteString("fuzzy:" + s.Fuzzy.Key() + ":" + fmt.Sprintf("%v", s.Opt.FuzzyAuto) + "|")
	}
	writeLocks(&b, s.locks)
	for i := range s.syls {
		if s.literals[i] {
			b.WriteString("literal:")
			b.WriteString(strconv.Itoa(i))
			b.WriteByte(';')
		}
	}
	return b.String()
}

// writeLocks 依位置順序寫入鎖定的全部欄位。
func writeLocks(b *strings.Builder, locks map[int]decode.Lock) {
	for _, st := range slices.Sorted(maps.Keys(locks)) {
		l := locks[st]
		b.WriteString(strconv.Itoa(st))
		b.WriteByte(':')
		b.WriteString(l.Word)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(l.Len))
		b.WriteByte(':')
		b.WriteString(strconv.FormatBool(l.Completion))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(l.LiteralBoundary))
		b.WriteByte(':')
		b.WriteString(l.Matched)
		b.WriteByte(';')
	}
}

func (s *Session) refresh() {
	for at, lock := range s.locks {
		boundary := at - 1
		if lock.LiteralBoundary > 0 {
			boundary = at + lock.Len
		}
		if lock.LiteralBoundary != 0 && !s.literals[boundary] {
			delete(s.locks, at) // 分隔英數已刪除，恢復正常解碼；其他標點不影響此邊界。
		}
	}
	if sig := s.signature(); sig != s.pathSig {
		s.dec.Beam, s.dec.Continue = s.Opt.Beam, s.Opt.Continue
		s.dec.SeparateContinuation = s.Opt.SeparateContinuation
		s.dec.PrefixCacheKeys = s.Opt.PrefixCacheKeys
		s.path = s.decodeWithLiterals()
		s.rerankPath()
		s.autoFuzzy()
		s.applyPronounHabit()
		s.shownPath, s.shownSyls = slices.Clone(s.path), slices.Clone(s.syls)
		s.nearShown = nil
		if len(s.path) == 0 && s.Fuzzy.Enabled() && len(s.syls) > 0 {
			s.nearShown = s.decodeAroundUnreadable()
		}
		if len(s.path) > 0 {
			s.displayed = map[int]string{}
			for _, g := range s.path {
				for i, r := range []rune(g.Word) {
					if !s.literals[g.Start+i] {
						s.displayed[g.Start+i] = string(r)
					}
				}
			}
		}
		s.pathSig, s.pathGen = sig, s.model.Gen
	}
	if len(s.path) > 0 {
		s.nearReading = false
	}
	s.sug = nil
	if !s.english && s.mixed.raw == "" && s.cursor == len(s.syls) && s.comp.Empty() && s.cand == nil && !s.autoFuzzed() {
		// refresh 每鍵最多跑三次；輸入沒變就沿用上次的建議。
		key := s.sugKey()
		if key == s.sugPrevKey && !s.noCaches {
			s.sug = s.sugPrev
		} else {
			s.sug = s.suggestAfterLiteral()
			s.sugPrev, s.sugPrevKey = s.sug, key
		}
		if s.sug != nil && s.revisitAwaits(s.sug.Start) {
			s.sug = nil // up␣ 的 因 不補成 因為：那個字之後可能換回 up
		}
	}
	s.refreshPhoneticHint()
}

// sugKey 是補全搜尋的輸入摘要。pathSig 在神經模式不含個人版本（#101），所以 Gen 與 Store.Time 另列。
func (s *Session) sugKey() string {
	var b strings.Builder
	b.WriteString(s.pathSig)
	b.WriteByte('|')
	writePath(&b, s.path) // refreshSelection 會在 pathSig 不變時換掉 path
	b.WriteString(strconv.Itoa(s.model.Gen))
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(s.model.Store.Time, 10))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(s.cursor))
	return b.String()
}

// writePath 寫入 path 的全部欄位。
func writePath(b *strings.Builder, path []decode.Seg) {
	for _, g := range path {
		b.WriteString(strconv.Itoa(g.Start))
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(g.End))
		b.WriteByte(',')
		b.WriteString(g.Word)
		if g.Partial {
			b.WriteByte('p')
		}
		if g.Correction {
			b.WriteByte('c')
		}
		if g.Matched != "" {
			b.WriteByte('#')
			b.WriteString(g.Matched)
		}
		b.WriteByte(' ')
	}
	b.WriteByte('|')
}

func (s *Session) openCandidates(length int) {
	s.styleGrid = false
	if s.englishWord {
		s.englishDismissed = false
		s.cand = nil
		s.refreshEnglishCandidates()
		return
	}
	symbol := s.mixedTrailingSymbol()
	if symbol {
		s.flushMixed() // hello< 的 ↓ 是游標前那個 < 的候選（第二項是全形「，」），不是整串
	}
	if s.mixed.raw != "" {
		s.openMixedCandidates()
		return
	}
	if !s.comp.Empty() && !s.finishUntoned() {
		s.openSymbols([]string{s.comp.String()}, s.cursor, 0, true)
		return
	}
	if len(s.syls) == 0 {
		return
	}
	before := symbol || s.candidateBefore()
	pos := min(s.cursor, len(s.syls)-1)
	if before {
		pos = max(0, s.cursor-1)
	}
	if s.literals[pos] {
		s.openLiteralAt(pos)
		return
	}
	anchor := pos
	if before {
		anchor = pos + 1
	}
	if length == 0 {
		length = 1 // ↓ 一律先開游標旁那一個字（我｜說不上 是「說」、說｜不上 是「不」），Tab 才換更長（P8）
	}
	s.openAnchored(anchor, length, before)
}

// openLiteralAt 開 pos 那格照字面文字的候選：原樣音節的各聲調、英數組得出的字（openRunFlips）、符號的全半形與原按鍵。
func (s *Session) openLiteralAt(pos int) {
	if p, ok := s.syllableLiteralAt(pos); ok {
		s.openSyllableLiteral(p)
		return
	}
	if s.openRunFlips(pos) {
		return
	}
	start := s.literalFloor(pos)
	end := s.literalStep(start, 1)
	var text strings.Builder
	for _, r := range s.syls[start:end] {
		text.WriteString(string(r))
	}
	alternatives := literalAlternatives(text.String())
	s.openSymbols(s.literalKeyAlternatives(alternatives, start, end-start), start, end-start, false)
}

// stepCandidate 把要修改的字往前（d=-1，j）或往後（d=1，k）換一個並重開候選（#193，照新酷音）：
// 不必 Esc、←／→、再 ↓。換到的是緊鄰目前候選範圍的那個字（標點等原樣文字整段算一個），
// 游標放到「修改字詞的位置」那一側再照 ↓ 的規則開窗（前方模式在字後、後方模式在字前）。
// ↓ 一律先開單字（P8），所以一次換一個字（官方 BD-0e2d73 一次換一個詞是配合 ↓ 先開現有分詞）。
// 後方模式在句尾開的前方候選也照後方模式往前換。已在頭尾時不動。
// 只用於改字的候選：Emoji／顏文字、中英混打的按鍵候選、注音打到一半的符號候選都不套用。
func (s *Session) stepCandidate(d int) {
	if !s.canStepCandidate() {
		return
	}
	start, end := s.cand.start, s.cand.start+s.cand.length
	if d < 0 && start == 0 || d > 0 && end >= len(s.syls) {
		return
	}
	s.refresh()
	switch {
	case d < 0 && s.CandidateBeforeCursor:
		s.cursor = start
	case d < 0:
		s.cursor = s.literalFloor(start - 1)
	case s.CandidateBeforeCursor:
		s.cursor = s.literalStep(s.literalFloor(end), 1)
	default:
		s.cursor = end
	}
	s.openCandidates(0)
}

// canStepCandidate：目前的候選是改字的候選，j／k 會換前後一個字（stepCandidate）。快照的 StepKeys 也依它判斷，
// 外殼的「J K 換字」提示跟實際行為一致（#193）。
func (s *Session) canStepCandidate() bool {
	c := s.cand
	return c != nil && c.kind == "" && !c.direct && !c.mixed && !c.english && !c.pending && c.length > 0 &&
		s.comp.Empty() && s.mixed.raw == "" && !s.englishWord
}

// candidateBefore 決定這次開窗選游標前方還是後方的詞。後方模式的游標在組字尾端時後面沒有字，
// 照字面只能選最後一個字，Tab 與上下鍵也換不到較長的詞；打完一句回頭改最後一個詞很常見，
// 所以這時改用前方模式的錨點，選結束在游標的詞。方向記在候選裡，換詞長時維持同一個錨點。
func (s *Session) candidateBefore() bool {
	return s.CandidateBeforeCursor || s.cursor >= len(s.syls)
}

// openAnchored 保留查詢方向；切換詞長時不把前方模式的尾端當成詞首。每份清單只列這個詞長的候選
// （拿掉官方 #161 接在詞後面的單字）。
func (s *Session) openAnchored(anchor, length int, before bool) bool {
	pos := anchor
	if before {
		pos -= length
	}
	if !s.openAt(pos, length) {
		return false
	}
	s.cand.before = before
	return true
}

func (s *Session) openAt(pos, length int) bool {
	if pos < 0 || length <= 0 || pos+length > len(s.syls) {
		return false
	}
	for i := pos; i < pos+length; i++ {
		if s.literals[i] {
			return false
		}
	}
	s.refresh()
	pinned := func(string) bool { return false }
	if length == 1 {
		syl := s.syls[pos]
		pinned = func(w string) bool { return s.model.PickedSingle(syl, w) || s.model.VocabReading(w, s.syls[pos:pos+1]) }
	} else if reading := s.syls[pos : pos+length]; s.model.HasVocab() {
		pinned = func(w string) bool { return s.model.VocabReading(w, reading) } // 自訂詞保證在第一頁（#122）
	}
	// 標點／原樣注音界定上下文，不讓候選評分跨到另一段。
	begin, end := pos, pos+length
	for begin > 0 && !s.literals[begin-1] {
		begin--
	}
	for end < len(s.syls) && !s.literals[end] {
		end++
	}
	locks := s.locksIn(begin, end)
	s.configureDocumentContext(begin)
	s.dec.Evidence = s.localEvidence(begin)
	s.configureCorrections(begin, end, true)
	// 與視窗相交的跨音鎖定即將被這次選取取代：不補它們的讀音，原音候選只依打出的音查詢。
	lat := s.pathCorrections(s.lockLattice(s.expand(s.syls), pos, pos+length), pos, pos+length)
	windows := []decode.Window{{Lat: lat[begin:end], Pinned: pinned}}
	if near, ok := s.nearWindow(lat, pos, length); ok {
		near.Lat = near.Lat[begin:end]
		windows = append(windows, near) // 近音版本與原音版本共用同一份前後文計算
	}
	if repair, ok := s.phoneticWindow(lat, pos, length); ok {
		repair.Lat = repair.Lat[begin:end]
		windows = append(windows, repair)
	}
	found := s.dec.CandidatesInContextWindows(lat[begin:end], pos-begin, length, locks, windows...)
	items := found[0]
	s.orderPronounCandidates(items, pos, length, pinned)
	var matched []string
	var near []decode.Candidate
	var nearMatched []string
	for _, group := range found[1:] {
		// Original readings win ties in provenance; dialect suggestions keep
		// their order, and duplicate new suggestions consume no extra slots.
		known := append(slices.Clone(items), near...)
		add, readings := s.nearOnly(group, known, pos, length)
		near = append(near, add...)
		nearMatched = append(nearMatched, readings...)
	}
	if len(near) > 0 {
		items, matched = mergeFuzzy(items, near, nearMatched)
	}
	// 第一個是畫面上的字、反白在它上面，↓ 直接 Enter 畫面不變；其他照統計（P8：官方照統計排，畫面上是神經選的字時不在第一個）。
	// 畫面上的字不是詞時（說不上 的「說不」）只在畫面上同一個詞裡才補上；只有近音的詞長不補（開模糊音不改原音候選）。
	exact := len(items) > 0 && (matched == nil || slices.Contains(matched, ""))
	added := false // 補上的不是詞（說不）：選了照畫面不動，不鎖也不學
	if shown, inWord, ok := s.shownWord(pos, length); ok && (exact || inWord) {
		items, matched, added = shownFirst(items, matched, shown, s.syls[pos:pos+length])
	}
	if len(items) == 0 && length > 1 {
		return false // 沒有詞的詞長（果事物、億美元、賺快）Tab 跳過，不開只有原按鍵的清單
	}
	// Keep physical keys recoverable without creating Chinese-word preferences.
	// The ordinary raw-zhuyin candidate remains last for existing clients.
	// 排在畫面上的字後面的：神經判過的字（趴 的 q8、教 的 url，P8 翻邊）；只用數字鍵出的字（#134 的 5 之、#162 的 20 單）
	// 要數字的人多；照位置組出的字（#274：i16 博、k27 的、u2/4 定）是引擎替使用者改的順序，同音字多時原按鍵在最後一頁找不到；
	// 型號後面數字鍵開頭的字（#249：h264 的、Q2 的）前面的數字也可能還是型號。再來是神經判過的字的其他切法（斷 → 2萬 → 段…）。
	insertRaw := func(at int, text string) {
		items = slices.Insert(items, at, rawCandidate(text))
		if matched != nil {
			matched = slices.Insert(matched, min(at, len(matched)), "")
		}
	}
	head := min(1, len(items))
	keyLiterals, flipAt := map[int]string{}, map[int]flipPick{}
	if added {
		flipAt[0] = flipPick{keep: true}
	}
	keys := s.originalMixedKeys(pos, length)
	front := keys != "" && (length == 1 && (s.judgedKeysSecond(pos) || s.digitWordKeys(pos) || s.digitAfterAlnumKeys(pos)) || s.positionalKeysIn(pos, length))
	if front {
		keyLiterals[head] = keys
		insertRaw(head, keys)
		head++
	}
	if length == 1 {
		for _, f := range s.chooseFlipChars(s.judgedSplits(pos)) {
			flipAt[head] = f
			insertRaw(head, s.mixedLiteralText(f.prefix)+f.char)
			head++
		}
	}
	if keys != "" && !front {
		keyLiterals[len(items)] = keys
		insertRaw(len(items), keys)
	}
	if length == 1 {
		insertRaw(len(items), s.syls[pos].Text())
	}
	s.cand = &candState{start: pos, length: length, items: items, raw: length == 1, matched: matched, keyLiterals: keyLiterals, flips: flipAt}
	s.cand.labels = s.candidateLabels(s.cand)
	s.labelSpatial(pos)
	return true
}

// shownWord 是畫面上 [pos, pos+length) 的字，inWord 表示整段在畫面上同一個詞裡；跨音、修正、
// 字數跟音節數不同（尾端補全）的詞不能照原樣鎖定，回 false。
func (s *Session) shownWord(pos, length int) (shown string, inWord, ok bool) {
	var text []rune
	segs := 0
	for _, g := range s.path {
		if g.End <= pos || g.Start >= pos+length {
			continue
		}
		word := []rune(g.Word)
		if g.Correction || g.Matched != "" || len(word) != g.End-g.Start {
			return "", false, false
		}
		text = append(text, word[max(0, pos-g.Start):min(len(word), pos+length-g.Start)]...)
		segs++
	}
	return string(text), segs == 1, len(text) == length
}

// shownFirst 把畫面上的字 shown 移到第一個（有原音的那一項就用它，只有近音就用近音那一項）；
// 不在清單裡（說不上 的「說不」不是詞）時補上，added 是 true。
func shownFirst(items []decode.Candidate, matched []string, shown string, reading []zhuyin.Syllable) (_ []decode.Candidate, _ []string, added bool) {
	at := -1
	for i, c := range items {
		if c.Entry.Surface != shown {
			continue
		}
		if at < 0 {
			at = i
		}
		if i >= len(matched) || matched[i] == "" {
			at = i
			break
		}
	}
	if at == 0 {
		return items, matched, false
	}
	c, m := decode.Candidate{Entry: lexicon.Entry{Surface: shown, Reading: slices.Clone(reading)}}, ""
	if at > 0 {
		c = items[at]
		items = slices.Delete(items, at, at+1)
		if at < len(matched) {
			m = matched[at]
			matched = slices.Delete(matched, at, at+1)
		}
	}
	items = slices.Insert(items, 0, c)
	if matched != nil {
		matched = slices.Insert(matched, 0, m)
	}
	return items, matched, at < 0
}

func (s *Session) pages() int { return (len(s.cand.items) + s.pageSize() - 1) / s.pageSize() }

// 候選開啟時 Tab 切至下一個較長詞長（d=1），最長後回到最短；Shift+Tab 反過來（d=-1）。不接受補全或提交。
// 同一串按鍵的清單（修改「2萬」）照「修改字詞的位置」只往一個方向包：前方模式（含句尾）從 萬 的單字多一步，
// 後方模式從前段的照字面清單（市值|2萬 的 2 / ㄉ）換過去，再 Tab 回來；市值2|萬 兩種模式都不包 2。
func (s *Session) cycleCandidateLength(d int) {
	c := s.cand
	if c == nil || c.mixed {
		return
	}
	if c.span != nil && c.span.literal {
		s.openLiteralAt(c.span.at)
		return
	}
	if c.literal {
		if q, ok := s.flipFromLiteral(); ok {
			s.openFlipSpan(q, spanBack{literal: true, at: c.start})
		}
		return
	}
	anchor, before := c.start, c.before
	maxLen := min(decode.MaxLen(s.dec.Lex), len(s.syls)-anchor)
	if before {
		anchor = c.start + c.length
		maxLen = min(decode.MaxLen(s.dec.Lex), anchor)
	}
	// 步驟 0 是單字，1 是同一串按鍵的清單，k ≥ 2 是 k 個字的詞。
	step := c.length
	if c.span != nil {
		step = 1
	} else if c.length == 1 {
		step = 0
	}
	n := maxLen + 1
	for i := 1; i <= n; i++ {
		next := ((step+d*i)%n + n) % n
		if next == 1 && before && s.openFlipSpan(anchor-1, spanBack{}) || next != 1 && s.openAnchored(anchor, max(next, 1), before) {
			return
		}
	}
}

// moveHighlight 把反白往下（d=1）或往上（d=-1）移一項，在同一份清單裡頭尾繞圈、不跨詞長（P8；
// 官方走過尾端會接到別的詞長）。換詞長用 Tab。
func (s *Session) moveHighlight(d int) {
	c := s.cand
	c.cursor = (c.cursor + d + len(c.items)) % len(c.items)
}

// selectCandidate 鎖定選取的詞。手動選取是硬條件；被換掉的原區段記下來供學習使用。
func (s *Session) selectCandidate(id int) bool {
	if s.cand != nil && s.cand.ids != nil {
		id = slices.Index(s.cand.ids, id)
	}
	if s.cand == nil || id < 0 || id >= len(s.cand.items) {
		return false
	}
	if s.cand.english {
		s.mixed.accepted = s.cand.items[id].Entry.Surface
		s.cand = nil
		return true
	}
	if s.cand.mixed {
		return s.selectMixed(id)
	}
	if f, ok := s.cand.flips[id]; ok {
		return s.selectFlip(f)
	}
	if s.cand.literal || s.cand.keyLiterals[id] != "" || s.cand.raw && id == len(s.cand.items)-1 {
		return s.selectLiteral(id)
	}
	start, length := s.cand.start, s.cand.length
	c, end := s.cand.items[id].Entry, start+length
	lock := decode.Lock{Word: c.Surface, Len: length}
	if id < len(s.cand.matched) {
		lock.Matched = s.cand.matched[id] // 跨音手選：鎖定連同它的讀音，近音路徑持續到鎖定解除
	}
	if s.AdvanceAfterSelect {
		if s.cand.before {
			s.cursor = s.literalStep(end, 1)
		} else {
			s.cursor = end
		}
	}
	s.cand = nil
	s.lockSelection(start, lock, c.Correction)
	return true
}

// lockSelection 鎖定手選的詞並重新解碼；被換掉的原區段記下來供學習使用。
func (s *Session) lockSelection(start int, lock decode.Lock, correction bool) {
	end := start + lock.Len
	shown, auto := slices.Clone(s.path), slices.Clone(s.autoLog)
	for _, g := range s.path {
		if g.Start <= start && start < g.End && (g.Word != lock.Word || g.Start != start || g.End != end) {
			s.rejected = append(s.rejected, personal.RejectedSeg{Word: g.Word, Start: g.Start, End: g.End, Correction: g.Correction})
		}
	}
	for st, l := range s.locks {
		if st < end && start < st+l.Len {
			delete(s.locks, st)
		}
	}
	s.locks[start] = lock
	s.refreshSelection(shown, auto, decode.Seg{Word: lock.Word, Start: start, End: end, Correction: correction, Matched: lock.Matched})
}

// fixWord 是不經候選視窗的手選：觸控從句首清單點詞時，外殼已經知道詞與位置，
// 再開一次該詞長的候選要帶著右邊整句重新評分，手機上要好幾秒。效果與從候選選同一個詞相同；
// 詞放不進這幾個音節（例如只靠近音才對得上）時什麼都不改，回報失敗，外殼改走候選。
func (s *Session) fixWord(start, length int, word string) bool {
	end := start + length
	if start < 0 || length <= 0 || utf8.RuneCountInString(word) != length {
		return false
	}
	// 失敗時整個還原，包括下面先完成的音節：未打完的注音不能因為一次放不進去的 fix 就被收掉（同 PreviewPending）。
	saved := *s
	s.syls, s.locks = slices.Clone(s.syls), maps.Clone(s.locks)
	s.literals, s.displayed = maps.Clone(s.literals), maps.Clone(s.displayed)
	s.rejected, s.autoLog = slices.Clone(s.rejected), slices.Clone(s.autoLog)
	fail := func() bool {
		*s = saved
		return false
	}
	s.finishUntoned() // 同送出與開候選：最後一個沒打聲調的音節先完成
	if end > len(s.syls) || !s.comp.Empty() {
		return fail()
	}
	for at := start; at < end; at++ {
		if s.literals[at] {
			return fail()
		}
	}
	fits, correction := s.dec.Fits(s.lockLattice(s.expand(s.syls), start, end), start, word)
	if !fits {
		return fail()
	}
	s.cand = nil
	s.lockSelection(start, decode.Lock{Word: word, Len: length}, correction)
	for _, g := range s.path {
		if g.Start == start && g.End == end && g.Word == word {
			return true
		}
	}
	return fail()
}

// accept 接受 Tab 補全：補上尚未輸入的音節並鎖定整個詞。接受是明確操作，
// 學習時保留補全來源，權重低於主動手選，避免系統建議自我強化。
func (s *Session) accept() bool {
	if s.englishWord {
		return s.acceptEnglish()
	}
	if s.mixed.raw != "" {
		return s.acceptMixed()
	}
	if s.phonetic != nil {
		return s.acceptPhonetic()
	}
	if s.sug == nil {
		return false
	}
	g := s.sug
	// 即使上層誤留建議，也不能跨越使用者鎖定的詞。
	for at, lock := range s.locks {
		if at < len(s.syls) && g.Start < at+lock.Len {
			return false
		}
	}
	s.syls = append(s.syls, g.Syls...)
	s.locks[g.Start] = decode.Lock{Word: g.Word, Len: len(s.syls) - g.Start, Completion: true}
	s.cursor = len(s.syls)
	return true
}

// commitComposed 是外殼替使用者結束組字（commit 請求：失焦、切到別的輸入法、App 結束組字、交給 App 的快捷鍵）
// 的送出：已組好的字照常送出，未打完的注音丟掉，不當成文字留在文件裡（#322，
// 同單按 Shift 切英文的 #90）。聲調選填時已是完整音節的注音照解碼送出（#150）；中英混打顯示成英文的暫存按鍵照畫面送出。
// 使用者自己按 Enter 仍照畫面原樣送出（commit）。
func (s *Session) commitComposed() string {
	s.finishUntoned()
	s.clearPendingZhuyin()
	return s.commit()
}

func (s *Session) commit() string {
	s.settleShown()   // never accept a suggestion on Enter
	s.finishUntoned() // 聲調選填：最後一個沒打聲調的音節也照解碼結果送出（#150）
	if s.Rerank.Scorer == nil {
		s.refresh()
	} // 神經模式提交已顯示路徑；Enter 不啟動推論或接受其他欄位的模型更新。
	text := s.snapshot().Text() // 未完成注音可能在句中，依畫面位置提交。
	// 沒打聲調的音節換回詞庫讀音再學（#150）；換不回來的詞不學（同近音），但包在裡面的手選用自己的讀音學。
	syls, _ := s.resolvedSyllables()
	ev := personal.Commit{Syls: s.lockReadings(syls), Rejected: s.rejectedReadings()}
	for _, g := range s.path {
		if s.literals[g.Start] {
			continue // 留下位置間隙，學習模型會在符號後重設上下文
		}
		l, locked := s.locks[g.Start]
		if l.LiteralBoundary != 0 {
			continue // 臨時保留的詞片段不是使用者選字，也不當作完整詞學習。
		}
		ev.Path = append(ev.Path, personal.Seg{
			Word: g.Word, Start: g.Start, End: g.End, Partial: g.Partial,
			Manual:     locked && l.Word == g.Word && l.Len == g.End-g.Start,
			Completion: locked && l.Word == g.Word && l.Len == g.End-g.Start && l.Completion,
			Suggested:  locked && l.Suggested,
			Fuzzy:      s.fuzzyTouches(g.Start, g.End) || untonedTouches(syls, g.Start, g.End),
		})
	}
	ev.Selected = s.enclosedSelections(ev.Syls)
	learn := !s.paused && !s.private && !s.learningCleared && !s.NoLearning
	s.lastLearn = nil // 這次沒學習時，undo_learn 也不能回頭還原更早那次提交
	if learn && len(ev.Path) > 0 {
		s.lastLearn = s.model.Undoable(func() { s.model.Learn(ev) })
	}
	// 英文學習跟著混打或英文模式自動補字走（#247）。
	if (s.mixedInput() || s.EnglishAutocomplete) && learn && !s.localBlocked {
		s.model.LearnEnglish(s.contextualEnglish())
	}
	if s.mixedInput() && learn && !s.localBlocked {
		s.model.LearnZhuyinKeys(s.zhuyinKeyCommits())
	}
	text = s.punctuate(text)
	tail := s.committedTail()
	neural := s.neuralCommitContext(text)
	s.reset()
	if tail != "" && text != "" {
		s.pendingLeft = tail
		s.pendingContextID = uint64(s.rev + 1)
	}
	if neural != "" {
		s.pendingNeural = neural
		s.pendingContextID = uint64(s.rev + 1)
	}
	return text
}

// punctuate 在提交時補標點。學習已依原路徑完成，插入的標點只影響送出的文字與之後的前文。
// 評分器失敗或回傳不合規的編輯時，原文照送。
func (s *Session) punctuate(text string) string {
	if s.Punct == nil || text == "" || s.private || s.paused || s.english {
		return text
	}
	edits, err := s.Punct.Punctuate(text)
	if err != nil {
		return text
	}
	out, _ := punct.Apply(text, edits)
	return out
}

func (s *Session) reset() {
	s.styleCycle = nil
	s.englishWord, s.englishDismissed, s.englishTokenBlocked = false, false, false
	s.paletteUndo = nil
	s.styleGrid = false
	s.mark = nil
	s.phonetic, s.phoneticUndo = nil, nil
	s.phoneticCache = phoneticHintCache{}
	s.paletteCommit = false
	s.clearMixed()
	s.mixedKeys = nil
	s.mixedOrigins = nil
	s.mixedRevisits = nil
	s.pendingToneKey, s.literalKeys = nil, nil
	s.toneKeyDone = ""
	s.clearDocumentContext()
	s.rerankCache = nil
	s.rerankMemo = nil
	s.sugPrev, s.sugPrevKey = nil, ""
	s.hint = nil
	s.shownPath, s.shownSyls = nil, nil
	s.private = false
	s.localBlocked = false
	s.learningCleared = false
	s.comp.Reset()
	s.syls, s.cursor, s.locks, s.rejected, s.cand = nil, 0, map[int]decode.Lock{}, nil, nil
	s.spatial, s.compNear, s.pendingSpatial = nil, nil, nil
	s.literals = nil
	s.pendingLiterals = nil
	s.displayed = nil
	s.nearShown = nil
	s.nearReading = false
	s.autoLog = nil
}

func (s *Session) snapshot() proto.Snapshot {
	snap := proto.Snapshot{FullWidth: s.fullWidth, Revision: s.rev, EnglishAutocomplete: s.EnglishAutocomplete, EnglishPending: s.englishWord, EnglishCanStart: s.englishCanStart()}
	pending := s.comp.String()
	if s.mixed.raw != "" {
		pending = s.mixedText()
	}
	split := pending != ""
	if s.mixed.raw != "" {
		pending = s.mixedLiteralText(pending) // 全形時打到一半的英數也照全形顯示（insertMixedLiteral）
	}
	pendingSegment := proto.Segment{Text: pending, Style: proto.StylePending}
	for _, g := range s.path {
		style := proto.StyleNormal
		switch l, locked := s.locks[g.Start]; {
		case locked && l.Word == g.Word && l.LiteralBoundary == 0:
			style = proto.StyleLocked
		case g.Correction || g.Matched != "":
			style = proto.StyleCorrected
		case g.Partial:
			style = proto.StylePartial
		}
		runes := []rune(g.Word)
		if split && g.Start <= s.cursor && s.cursor < g.End {
			k := s.cursor - g.Start
			if k > 0 {
				snap.Preedit = append(snap.Preedit, proto.Segment{Text: string(runes[:k]), Style: style})
			}
			snap.Preedit = append(snap.Preedit, pendingSegment)
			snap.Preedit = append(snap.Preedit, proto.Segment{Text: string(runes[k:]), Style: style})
			split = false
			continue
		}
		snap.Preedit = append(snap.Preedit, proto.Segment{Text: g.Word, Style: style})
	}
	before := s.cursor // 游標之前的字數
	if len(s.path) == 0 && len(s.syls) > 0 {
		// 保底：英數切開詞條時保留已顯示漢字，其餘無法解碼的音節顯示原樣注音。
		// 組字區絕不空白，游標依實際顯示字數計算。
		before = 0
		for i, y := range s.syls {
			text, style := s.fallbackDisplay(i, y)
			if split && i == s.cursor {
				snap.Preedit = append(snap.Preedit, pendingSegment)
				split = false
			}
			if i < s.cursor {
				before += len([]rune(text))
			}
			snap.Preedit = append(snap.Preedit, proto.Segment{Text: text, Style: style})
		}
	}
	if split {
		snap.Preedit = append(snap.Preedit, pendingSegment)
	}
	if start, end := s.markRange(); start < end && pending == "" {
		snap.Preedit = markSegments(snap.Preedit, start, end) // 沒有未完成注音時，音節與字元一一對應
	}
	// 已解碼音節與字面符號各占一個字元，游標在未完成注音之後。
	runes := []rune(snap.Text())
	snap.Cursor.Rune = min(before+len([]rune(pending)), len(runes))
	snap.Cursor.UTF16 = len(utf16.Encode(runes[:snap.Cursor.Rune]))
	// 小視窗的 Tab 提示是按了之後組字區結尾變成的樣子（另 → 另外、png → ㄕㄣ），SuggestionEdit 是被換掉的範圍。
	// 前方可能含多字元原樣注音，不能直接把音節位置當顯示字元位置，一律從結尾往回算。
	tail := func(n int, text string) { snap.SetSuggestion(len(runes)-n, n, text) }
	if h := s.phonetic; h != nil {
		tail(len([]rune(h.replace)), h.entry.Surface)
	} else if g := s.sug; g != nil {
		tail(len([]rune(g.Word))-len([]rune(g.Rest)), g.Word)
	} else if h := s.hint; h != nil && s.mixed.raw == "" && s.cursor == len(s.syls) && s.comp.Empty() && s.cand == nil {
		tail(len([]rune(h.replace)), h.text) // 神經次佳候選，按 Tab 才接受
	}
	if s.cand != nil {
		cl := &proto.CandidateList{Page: s.candidatePage(), Pages: s.pages(), Start: s.cand.start, Length: s.cand.length, Highlight: s.cand.cursor % s.pageSize(), Columns: s.candidateColumns(), Kind: s.cand.kind, PageSize: s.pageSize()}
		if cl.Columns == 1 {
			cl.Columns = 0
		}
		if s.cand.english {
			cl.Title, cl.PageSize = "按 Tab 補全或修正拼字 · Esc 關閉", englishCandidateLimit
		}
		if cl.Kind != "" {
			cl.Title = s.paletteTitle()
		}
		// 照字面清單只有 Tab 換得到同一串按鍵的清單時才有標題（市值|2萬 的 2 / ㄉ）。
		if _, span := s.flipFromLiteral(); !s.cand.mixed && cl.Length > 0 && (!s.cand.literal || span) {
			cl.Title = "修改「" + s.candidateText() + "」 · Tab 切換詞長"
		}
		// 選字鍵含 j、k（asdfghjkl;）時它們是選字，不是換字。
		cl.StepKeys = s.canStepCandidate() && s.selectionIndex("j") < 0 && s.selectionIndex("k") < 0
		lo := s.candidatePage() * s.pageSize()
		for id := lo; id < min(lo+s.pageSize(), len(s.cand.items)); id++ {
			item := proto.CandidateItem{ID: s.candidateID(id), Text: s.cand.items[id].Entry.Surface}
			if s.cand.kind == "phrases" {
				item.Text = phraseLabel(item.Text)
			}
			if id < len(s.cand.matched) {
				item.Note = nearNote(s.cand.matched[id]) // 原音項目是空字串
			}
			if e := s.cand.items[id].Entry; item.Note == "" && s.cand.kind == "" && len(e.Reading) > 0 && s.model.VocabReading(e.Surface, e.Reading) {
				item.Note = "我的詞彙" // 讓人知道這是自己加的詞、按反悔鍵會刪掉（比照 Apple 注音標「常用」）
			}
			if s.cand.keyLiterals[id] != "" {
				item.Note = "原始按鍵"
			}
			if id < len(s.cand.labels) && s.cand.keyLiterals[id] == "" {
				item.Label = s.cand.labels[id]
			}
			if !s.cand.english { // 英文補字只能按 Tab 接受（#112）
				item.Key = string(s.selectionKeys()[id-lo])
			}
			cl.Items = append(cl.Items, item)
		}
		snap.Candidates = cl
	}
	s.mixedSnapshot(&snap)
	return snap
}

// Learned 列出目前學到的個人片語，依晉升順序。
func (s *Session) Learned() []string {
	out := append([]string(nil), s.model.Store.PhraseOrder...)
	sort.Strings(out)
	return out
}
