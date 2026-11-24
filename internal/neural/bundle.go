package neural

import (
	"os"
	"path/filepath"
	"runtime"
)

// SourceBundled 是模型與推論環境的來源（安裝包內附），health 的 selection_source／punctuation_source、neural_python 回報它。
const SourceBundled = "bundled"

// Bundled 是找到的推論環境與模型。HasSelect／HasPunct 表示可以啟動 worker（有模型、worker 與內附的 Python）。
// 內附 Python 不見是安裝損毀：Python 為空、不啟動 worker，狀態是 not_installed。
type Bundled struct {
	Python, SelectWorker, SelectPackage, PunctWorker, PunctPackage string
	PythonSource, SelectSource, PunctSource                        string // 空字串：沒有找到
	HasSelect, HasPunct                                            bool
}

// Find 分開找安裝包內附的 Python、worker 與模型；模型沒有時由統計引擎選字。
//
//	<data>/../neural/python/             內附 Python 與 numpy、onnxruntime
//	<data>/../neural/*_runtime.py        worker
//	<data>/../neural/select|punct/       內建的模型，與該字典配對出貨
func Find(dataDir string) Bundled {
	dir := filepath.Join(filepath.Dir(filepath.Clean(dataDir)), "neural")
	var b Bundled
	if python := filepath.Join(dir, "python", "python.exe"); regular(python) {
		b.Python, b.PythonSource = python, SourceBundled
	}
	find := func(worker, name string) (string, string, string) {
		script, pkg := filepath.Join(dir, worker), filepath.Join(dir, name)
		if regular(script) && regular(filepath.Join(pkg, "manifest.json")) {
			return script, pkg, SourceBundled
		}
		return script, pkg, ""
	}
	b.SelectWorker, b.SelectPackage, b.SelectSource = find("sequence_runtime.py", "select")
	b.PunctWorker, b.PunctPackage, b.PunctSource = find("punct_runtime.py", "punct")
	b.HasSelect = b.Python != "" && b.SelectSource != ""
	b.HasPunct = b.Python != "" && b.PunctSource != ""
	return b
}

func regular(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

// MaxThreads 是 worker 接受的 threads 上限，須與 Python worker 的檢查一致。
const MaxThreads = 8

// DefaultThreads 用滿 CPU 到上限為止：推論只在按鍵與提交時短暫執行，閒置時不占用。
func DefaultThreads() int {
	return max(1, min(MaxThreads, runtime.NumCPU()))
}
