package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/internal/models"
	upstream "github.com/0c-biggo/0c-ime/models"
)

// 「模型」頁只讀安裝包內建的模型：是 models.lock 那一份才附上 Hugging Face 的來源與授權，引擎在跑時帶上它回報的狀態。
func TestModelsStatusReportsBundledModels(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	neuralDir := filepath.Join(root, "neural")
	lock, err := models.ParseLock(upstream.Lock)
	if err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(neuralDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(data, 0o755)
	write(filepath.Join("python", "python.exe"), "")
	write("sequence_runtime.py", "")
	write("punct_runtime.py", "")
	selectSHA := lock.Models[models.RoleSelect].Files["model.onnx"].SHA256
	write(filepath.Join("select", "manifest.json"), `{"files":{"model.onnx":"`+selectSHA+`"}}`)
	write(filepath.Join("select", "model.onnx"), "weights")
	write(filepath.Join("punct", "manifest.json"), `{"files":{"model.onnx":"`+strings.Repeat("c", 64)+`"}}`)

	env := modelsEnv{data: data, lock: upstream.Lock, equivalent: upstream.Equivalent,
		dial: modelStatusDial(map[string]string{"version": "dev", "selection_state": "ready", "punctuation_state": "ready"}, true)}
	st, err := env.status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Source != "bundled" || len(st.Models) != 2 {
		t.Fatalf("status：%+v", st)
	}
	sel, punct := st.Models[0], st.Models[1]
	want := lock.Models[models.RoleSelect]
	if sel.Path != filepath.Join(neuralDir, "select") || sel.SHA256 != selectSHA || sel.Size != int64(len("weights")) || sel.State != "ready" ||
		sel.Repository != want.Repository || sel.Revision != want.Revision || sel.URL != want.URL() || sel.License != want.License || sel.Equivalent {
		t.Fatalf("選字：%+v", sel)
	}
	// 不是上游那一份（例如自己換的模型）：照樣顯示路徑與雜湊，不冒用上游的網址。
	if punct.Path != filepath.Join(neuralDir, "punct") || punct.URL != "" || punct.Repository != "" || punct.Version != "" {
		t.Fatalf("補標點：%+v", punct)
	}
	var text bytes.Buffer
	printModels(&text, st)
	if !strings.Contains(text.String(), "神經模型：安裝包內建") || !strings.Contains(text.String(), want.URL()) {
		t.Fatalf("文字輸出：%s", text.String())
	}

	// 安裝損毀、找不到內建模型：統計選字，不附任何上游資訊；引擎沒在跑時狀態留空。
	os.RemoveAll(neuralDir)
	env.dial = modelStatusDial(nil, false)
	if st, err = env.status(); err != nil || st.Source != "none" || st.Models[0].Path != "" || st.Models[0].URL != "" || st.Models[0].State != "" {
		t.Fatalf("沒有模型：%+v %v", st, err)
	}
}
