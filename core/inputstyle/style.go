// Package inputstyle 是按鍵規則：使用者可在設定改的幾個按鍵操作。核心與宿主共用同一份值；不含平台 I/O。
package inputstyle

const Version = 1

// Rules 是按鍵規則。ShiftSpace…CandidateNavigation 由設定決定（值見下面的常數），
// Punctuation、Up、ShiftLetter、CapsLetter 是官方風格留下、引擎目前寫死的值。
// 零值維持舊的按鍵處理（測試用）。
type Rules struct {
	CapsLetter          string `json:"caps_letter"`
	CandidateNavigation string `json:"candidate_navigation"`
	ShiftDelete         string `json:"shift_delete"`
	Up                  string `json:"up"`
	CandidateTab        string `json:"candidate_tab"`
	CandidatePageSize   int    `json:"candidate_page_size"`
	Version             int    `json:"version"`
	Punctuation         string `json:"punctuation"`
	ShiftSpace          string `json:"shift_space"`
	Tab                 string `json:"tab"`
	CandidateSpace      string `json:"candidate_space"`
	CandidateEnter      string `json:"candidate_enter"`
	ShiftLetter         string `json:"shift_letter"`
}

// 設定可選的值。
const (
	ShiftSpaceDisabled   = "disabled"   // Shift＋空白不動作
	ShiftSpaceFullWidth  = "fullwidth"  // 切換全半形（含英數）
	ShiftSpaceSpace      = "space"      // 插入空白
	ShiftSpaceSingle     = "single"     // 開單字候選
	TabAccept            = "accept"     // 接受補全與同音建議
	TabCycle             = "cycle"      // 輪換字詞
	CandidateTabLength   = "length"     // 切換字詞長度
	CandidateTabGrid     = "grid"       // 展開多排候選
	CandidateTabNext     = "next"       // 移到下一個候選
	CandidateSpacePage   = "page"       // 下一頁
	CandidateSpaceSelect = "select"     // 選取候選字
	CandidateEnterSelect = "select"     // 選字並保留組字
	CandidateEnterCommit = "commit"     // 選字並送出
	ShiftDeleteDelete    = "delete"     // 同 Delete
	ShiftDeleteTone      = "tone"       // 退回聲調
	NavigationLayout     = "layout"     // 依候選排列
	NavigationHorizontal = "horizontal" // 左右選候選、上下翻頁
)
