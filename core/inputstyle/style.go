// Package inputstyle 是按鍵規則：使用者可在設定改的幾個按鍵操作。核心與宿主共用同一份值；不含平台 I/O。
package inputstyle

// Rules 是按鍵規則，值見下面的常數。零值是各項的第一個值（Shift＋空白不動作、Tab 接受補全…），每頁 10 個候選。
type Rules struct {
	ShiftSpace          string
	Tab                 string
	CandidateTab        string
	CandidateSpace      string
	CandidateEnter      string
	CandidatePageSize   int
	ShiftDelete         string
	CandidateNavigation string
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
