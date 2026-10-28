package settings

import (
	"reflect"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
)

// officialRulesDefault 回報操作規則除了整合風格加的欄位（fork:"blend"）之外，是不是都是官方預設值。
// 官方只在預設規則下開放混打、英文補全、聲調選填與非標準排列；整合風格的欄位是為混打設計的，不算在內。
// 官方新增的規則先照官方的做法處理，由 forkcheck 測試提醒分類。
func officialRulesDefault(r inputstyle.Rules) bool {
	v := reflect.ValueOf(&r).Elem()
	for _, f := range reflect.VisibleFields(v.Type()) {
		if f.Tag.Get("fork") != "" {
			v.FieldByIndex(f.Index).SetZero()
		}
	}
	return r == inputstyleDefaultRules()
}
