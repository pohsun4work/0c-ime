package settings

import (
	"reflect"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
)

// officialRulesDefault 回報操作規則除了整合風格加的欄位（fork:"blend"）之外，是不是都是官方預設值。
// 官方的風格只在預設規則下開放非標準排列（profileAvailability）；整合風格的欄位不算在內。
// 跟混打、英文補全、聲調選填能不能並用由官方的 styleConflicts 逐條判斷，整合風格的欄位不在表上。
func officialRulesDefault(r inputstyle.Rules) bool {
	v := reflect.ValueOf(&r).Elem()
	for _, f := range reflect.VisibleFields(v.Type()) {
		if f.Tag.Get("fork") != "" {
			v.FieldByIndex(f.Index).SetZero()
		}
	}
	return r == inputstyleDefaultRules()
}
