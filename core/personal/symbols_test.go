package personal

import (
	"bytes"
	"strings"
	"testing"
)

// fork 拿掉了符號選單：官方格式 7 的 SymbolRecent／SymbolStats 讀得進來（其他資料照舊），存檔不再寫，版本號仍是 7。
func TestSymbolRecentIgnoredFormatUnchanged(t *testing.T) {
	s, err := LoadStore(strings.NewReader(`{"Format":7,"Uni":{"保留":2},"SymbolRecent":["※"],"SymbolStats":{"※":{"s":3}}}`))
	if err != nil || s.Uni["保留"] != 2 {
		t.Fatalf("%v %+v", err, s)
	}
	var b bytes.Buffer
	if err := s.Save(&b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b.Bytes(), []byte(`"Format":7`)) || bytes.Contains(b.Bytes(), []byte("Symbol")) {
		t.Fatalf("存成格式 7、不寫符號紀錄：%s", b.String())
	}
}
