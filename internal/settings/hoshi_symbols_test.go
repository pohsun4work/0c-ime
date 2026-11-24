package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// oldSymbolsFile 寫一個 P4 以前的設定檔：標點寬度全形、內建靈犀注音的快照標點操作是 0c，外加本版不認得的欄位。
func oldSymbolsFile(t *testing.T, toneless bool) (user, path string) {
	t.Helper()
	dir := t.TempDir()
	user = filepath.Join(dir, "user.json")
	path = Path(user)
	s := freshSettings()
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	members, err := orderedMembers(b)
	if err != nil {
		t.Fatal(err)
	}
	// 照原順序改寫（input_profile 在最後，讀到它時前面的欄位都已讀進來）。
	out := []string{`"future":42`}
	for _, m := range members {
		raw := []byte(m.raw)
		switch m.key {
		case "shift_symbol_halfwidth", "all_symbols_halfwidth":
			raw = []byte("false")
		case "toneless":
			raw, _ = json.Marshal(toneless)
		case "input_profile":
			var st profileStore
			if err := json.Unmarshal(raw, &st); err != nil {
				t.Fatal(err)
			}
			for i, p := range st.Items {
				if p.ID == "0c" {
					st.Items[i].Rules.Punctuation = "0c"
					if toneless {
						st.Items[i].Values["toneless"] = "on"
					}
				}
			}
			raw, _ = json.Marshal(st)
		}
		k, _ := json.Marshal(m.key)
		out = append(out, string(k)+":"+string(raw))
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, []byte("{"+strings.Join(out, ",")+"}"), "", "  "); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, indented.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return user, path
}

func builtin0cPunctuation(t *testing.T, path string) string {
	t.Helper()
	s, err := Load(path)
	if err != nil || s.ReadOnly() {
		t.Fatalf("讀回：%v %v", err, s.ReadOnly())
	}
	for _, p := range s.store().Items {
		if p.ID == "0c" {
			return p.Rules.Punctuation
		}
	}
	t.Fatal("沒有內建靈犀注音")
	return ""
}

func TestAdoptHoshiSymbols(t *testing.T) {
	user, path := oldSymbolsFile(t, false)
	if wrote, err := AdoptHoshiSymbols(user); err != nil || !wrote {
		t.Fatalf("第一次要改：%v %v", wrote, err)
	}
	s, _ := Load(path)
	if s.symbolWidth() != "all" || builtin0cPunctuation(t, path) != "microsoft" {
		t.Fatalf("標點寬度 %q、靈犀注音標點 %q", s.symbolWidth(), builtin0cPunctuation(t, path))
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), `"future": 42`) {
		t.Fatalf("不認得的欄位要留著：%s", b)
	}
	// 之後使用者自己改回去不會再被改掉。
	if err := s.Set("shift-symbol-halfwidth", "off"); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	if wrote, err := AdoptHoshiSymbols(user); err != nil || wrote {
		t.Fatalf("只換一次：%v %v", wrote, err)
	}
	if back, _ := Load(path); back.symbolWidth() != "off" {
		t.Fatalf("使用者改回的值被改掉：%q", back.symbolWidth())
	}
}

// 沒有設定檔（新安裝）：只記 marker，不建檔；新的預設本來就是這樣。
func TestAdoptHoshiSymbolsFreshInstall(t *testing.T) {
	user := filepath.Join(t.TempDir(), "user.json")
	if wrote, err := AdoptHoshiSymbols(user); err != nil || wrote {
		t.Fatalf("沒有設定檔：%v %v", wrote, err)
	}
	if _, err := os.Stat(HoshiSymbolsMarker(Path(user))); err != nil {
		t.Fatalf("要記 marker：%v", err)
	}
	if _, err := os.Stat(Path(user)); !os.IsNotExist(err) {
		t.Fatalf("不建設定檔：%v", err)
	}
	if Default().symbolWidth() != "all" {
		t.Fatal("新預設是所有符號鍵打半形")
	}
}

// 開著不打聲調的使用者標點操作、標點寬度都不改：兩者都會讓不打聲調停用。
func TestAdoptHoshiSymbolsKeepsTonelessProfile(t *testing.T) {
	user, path := oldSymbolsFile(t, true)
	if _, err := AdoptHoshiSymbols(user); err != nil {
		t.Fatal(err)
	}
	if s, _ := Load(path); s.symbolWidth() != "off" || !s.EffectiveToneless() || builtin0cPunctuation(t, path) != "0c" {
		t.Fatalf("標點寬度 %q、靈犀注音標點 %q", s.symbolWidth(), builtin0cPunctuation(t, path))
	}
}

// 沒在用的風格（存過的微軟新注音）不因排列讓設定檔變唯讀；官方 0c 標點的舊快照仍可配倚天。
func TestInactiveProfileDoesNotLockLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := freshSettings()
	for _, id := range []string{"microsoft", "0c"} {
		if err := s.ProfileAction("select", id, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Set("keyboard-layout", LayoutETen); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	if back, err := Load(path); err != nil || back.ReadOnly() || back.KeyboardLayout != LayoutETen {
		t.Fatalf("讀回：%v ro=%v %q", err, back.ReadOnly(), back.KeyboardLayout)
	}
	_, old := oldSymbolsFile(t, false)
	b, _ := os.ReadFile(old)
	b = []byte(strings.Replace(string(b), `"keyboard_layout": "standard"`, `"keyboard_layout": "eten"`, 1))
	if err := os.WriteFile(old, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if back, err := Load(old); err != nil || back.ReadOnly() || back.KeyboardLayout != LayoutETen {
		t.Fatalf("官方 0c 標點的舊快照配倚天：%v ro=%v %q", err, back.ReadOnly(), back.KeyboardLayout)
	}
}
