package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/0c-biggo/0c-ime/internal/filelock"
)

// HoshiSymbolsMarker 是已經換成 fork 符號預設的紀錄，放在設定檔旁的 update 目錄（同 BothShiftMarker）。
func HoshiSymbolsMarker(settingsPath string) string {
	return filepath.Join(filepath.Dir(settingsPath), "update", "hoshi-symbols")
}

// AdoptHoshiSymbols（fork P4）：符號照微軟。舊設定檔第一次以這一版啟動時換一次：
// 標點寬度改成「所有符號鍵打半形」，存下來的內建「靈犀注音」快照標點操作從 0c 改成微軟參考（新安裝的預設同此）；
// 開著不打聲調的使用者兩者都不改（不能並用）。
// 之後使用者自己改回去不會再被改掉。其他欄位（含本版不認得的）照原樣、照原順序寫回；
// 唯讀（較新版本寫的）檔案不動；讀不到或無法解析時回傳錯誤、不記 marker，下次再試。
func AdoptHoshiSymbols(userStore string) (bool, error) {
	path := Path(userStore)
	if _, err := os.Stat(HoshiSymbolsMarker(path)); err == nil {
		return false, nil
	}
	wrote := false
	err := filelock.With(filepath.Dir(userStore), WriteLock, func() error {
		b, missing, err := readFile(path)
		if err != nil {
			return err
		}
		if !missing {
			if wrote, err = adoptHoshiSymbols(path, b); err != nil {
				return err
			}
		}
		marker := HoshiSymbolsMarker(path)
		if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
			return err
		}
		return os.WriteFile(marker, []byte("p4\n"), 0o600)
	})
	return wrote, err
}

func adoptHoshiSymbols(path string, b []byte) (bool, error) {
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return false, fmt.Errorf("%s：%w：%v", path, ErrCorrupt, err)
	}
	if s.readOnly {
		return false, nil
	}
	members, err := orderedMembers(b)
	if err != nil {
		return false, fmt.Errorf("%s：%w：%v", path, ErrCorrupt, err)
	}
	var out bytes.Buffer
	out.WriteByte('{')
	put := func(key string, raw []byte) {
		if out.Len() > 1 {
			out.WriteByte(',')
		}
		k, _ := json.Marshal(key)
		out.Write(k)
		out.WriteByte(':')
		out.Write(raw)
	}
	for _, m := range members {
		switch m.key {
		case "shift_symbol_halfwidth", "all_symbols_halfwidth":
			if !s.Toneless {
				continue // 寫在最後
			}
		case "input_profile":
			if raw, ok := builtinMicrosoftPunctuation(m.raw); ok {
				put(m.key, raw)
				continue
			}
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, m.raw); err != nil {
			return false, fmt.Errorf("%s：%w：%v", path, ErrCorrupt, err)
		}
		put(m.key, compact.Bytes())
	}
	if !s.Toneless { // 開著不打聲調的不改：所有符號鍵打半形會讓它暫停
		put("shift_symbol_halfwidth", []byte("true"))
		put("all_symbols_halfwidth", []byte("true"))
	}
	out.WriteByte('}')
	var indented bytes.Buffer
	if err := json.Indent(&indented, out.Bytes(), "", "  "); err != nil {
		return false, fmt.Errorf("%s：%w：%v", path, ErrCorrupt, err)
	}
	indented.WriteByte('\n')
	if bytes.Equal(indented.Bytes(), b) {
		return false, nil
	}
	// 寫入前再核對一次：讀得回來、仍可寫、標點寬度是 all（開著不打聲調的除外）。
	var check Settings
	if json.Unmarshal(indented.Bytes(), &check) != nil || check.readOnly || !s.Toneless && check.symbolWidth() != "all" {
		return false, fmt.Errorf("%s：%w", path, ErrCorrupt)
	}
	return true, writeAtomic(path, indented.Bytes())
}

// builtinMicrosoftPunctuation 把存下來的內建「靈犀注音」快照的標點操作從 0c 改成微軟參考；
// 讀不懂或沒有要改的回傳 false，原樣寫回。
func builtinMicrosoftPunctuation(raw json.RawMessage) ([]byte, bool) {
	var st profileStore
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&st) != nil {
		return nil, false
	}
	changed := false
	for i, p := range st.Items {
		// 開著不打聲調的快照不改：它跟微軟參考不能並用（validateProfileCombination）。
		if p.Builtin && p.ID == "0c" && p.Rules.Punctuation == "0c" && p.Values["toneless"] != "on" {
			st.Items[i].Rules.Punctuation = "microsoft"
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	out, err := json.Marshal(st)
	return out, err == nil
}
