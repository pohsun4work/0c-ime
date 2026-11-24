package main

import (
	"path/filepath"
	"testing"

	"github.com/0c-biggo/0c-ime/internal/settings"
)

// writeOfficialPunctuationSettings 存一份換回官方標點操作（靈犀）與全形標點的設定檔，再套用 pairs。
// fork：0c 風格預設微軟參考、所有符號鍵打半形，不打聲調在那之下不能用或暫停。
func writeOfficialPunctuationSettings(t *testing.T, base string, pairs ...[2]string) {
	t.Helper()
	s := settings.Default()
	if err := s.ProfileAction("copy", s.ActiveProfile().ID, "官方標點"); err != nil {
		t.Fatal(err)
	}
	for _, kv := range append([][2]string{{"style-punctuation", "0c"}, {"shift-symbol-halfwidth", "off"}}, pairs...) {
		if err := s.Set(kv[0], kv[1]); err != nil {
			t.Fatalf("%v：%v", kv, err)
		}
	}
	if err := settings.Save(filepath.Join(base, settings.FileName), s); err != nil {
		t.Fatal(err)
	}
}

// #150：「不打聲調也能選字」經 settings --json 給設定程式；引擎載入的值不同時標示待套用，舊引擎沒回報時不猜。
// 開著時中英混打標成不起作用（與混打二選一），值照設定檔。
func TestTonelessContractReportsPendingAndDisablesMixedInput(t *testing.T) {
	base := t.TempDir()
	writeOfficialPunctuationSettings(t, base, [2]string{"mixed-input", "on"}, [2]string{"toneless", "on"})
	for _, tc := range []struct{ runtime, want string }{{"false", "off"}, {"true", "on"}, {"", ""}} {
		v, ok := runningValues(map[string]string{"toneless": tc.runtime})["toneless"]
		if v != tc.want || ok != (tc.runtime != "") {
			t.Fatalf("runtime %q: %q %v", tc.runtime, v, ok)
		}
		doc := buildSettingsDoc(base, modelStatusDial(map[string]string{"toneless": tc.runtime}, true))
		it := item(t, doc, "toneless")
		if it.Value != "on" || it.Pending != (tc.runtime == "false") || (it.Running != nil) != ok {
			t.Fatalf("pending contract: %+v", it)
		}
		if mixed := item(t, doc, "mixed-input"); mixed.Value != "on" || !mixed.Disabled || mixed.DisabledReason != "不打聲調時關閉" {
			t.Fatalf("混打反灰：%+v", mixed)
		}
	}
	if o, err := parseSetArgs([]string{"toneless", "on", "-apply", "-json"}); err != nil || len(o.pairs) != 1 || !o.apply {
		t.Fatalf("%+v %v", o, err)
	}
}
