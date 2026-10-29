//go:build blendall

package session

import (
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
)

// 整合風格下跑官方測試：go test -tags blendall ./core/session。每個 New 建的 session 一開始就套用整合風格的規則，
// 預期不同的測試列在 testdata/blend_expected_diffs.txt，由 scripts/blend_official_diff.py 核對。
// BLENDALL_OFF 列出要關掉的欄位（json 名稱，逗號分隔），用來找出差異是哪一條規則造成的。
func init() {
	p, _ := inputstyle.Get("blend")
	off := strings.Split(os.Getenv("BLENDALL_OFF"), ",")
	v := reflect.ValueOf(&p.Rules).Elem()
	for i := 0; i < v.NumField(); i++ {
		name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		if slices.Contains(off, name) {
			v.Field(i).SetZero()
		}
	}
	defaultStyle = p.Rules
}
