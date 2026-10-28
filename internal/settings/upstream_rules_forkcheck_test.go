//go:build forkcheck

package settings

import (
	"reflect"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
)

// 官方新增或改了操作規則時，提醒 fork 決定它跟混打相不相容（officialRulesDefault）。沒分類的規則照官方的
// 做法擋混打，不會出錯，所以 CI 合併官方新版後以 -tags forkcheck 單獨跑，失敗只開 issue、照常發布。
// 分類完把新的欄位或值補進下面兩張表；整合風格自己的欄位（fork:"blend"）不列。
func TestUpstreamRulesClassified(t *testing.T) {
	fields := "CapsLetter CandidateNavigation ShiftDelete Up CandidateTab CandidatePageSize Version Punctuation ShiftSpace Tab CandidateSpace CandidateEnter ShiftLetter"
	var got []string
	for _, f := range reflect.VisibleFields(reflect.TypeOf(inputstyle.Rules{})) {
		if f.Tag.Get("fork") == "" {
			got = append(got, f.Name)
		}
	}
	if strings.Join(got, " ") != fields {
		t.Errorf("官方改了操作規則的欄位：\n現在 %s\n原本 %s", strings.Join(got, " "), fields)
	}
	values := map[string]string{
		"style-shift-space":          "disabled fullwidth space single",
		"style-tab":                  "0c cycle segment",
		"style-up":                   "0c cycle",
		"style-candidate-tab":        "length grid next",
		"style-candidate-space":      "page select",
		"style-candidate-enter":      "select commit",
		"style-page-size":            "9 10",
		"style-punctuation":          "0c microsoft standard chewing",
		"style-shift-letter":         "inline commit",
		"style-shift-delete":         "0c tone",
		"style-caps-letter":          "platform lower",
		"style-candidate-navigation": "0c horizontal",
	}
	specs := ruleSpecs()
	for _, sp := range specs {
		if v := strings.Join(sp.values, " "); values[sp.key] != v {
			t.Errorf("官方改了規則 %s 的值：現在 %q，原本 %q", sp.key, v, values[sp.key])
		}
	}
	if len(specs) != len(values) {
		t.Errorf("官方的規則從 %d 條變成 %d 條", len(values), len(specs))
	}
}
