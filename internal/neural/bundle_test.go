package neural

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// 布局：root/data（字典）、root/neural（worker、內附模型、內附 Python）。
type findLayout struct{ root, data, neural string }

func newFindLayout(t *testing.T) findLayout {
	t.Helper()
	root := t.TempDir()
	l := findLayout{root: root, data: filepath.Join(root, "data"), neural: filepath.Join(root, "neural")}
	os.MkdirAll(l.data, 0o755)
	return l
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func bundledPython(neural string) string { return filepath.Join(neural, "python", "python.exe") }

// 官網安裝包：內附 Python、worker、模型都在 <data>/../neural。
func TestFindBundledLayout(t *testing.T) {
	l := newFindLayout(t)
	touch(t, bundledPython(l.neural))
	for _, p := range []string{"sequence_runtime.py", "punct_runtime.py", "select/manifest.json", "punct/manifest.json"} {
		touch(t, filepath.Join(l.neural, p))
	}
	b := Find(l.data)
	if !b.HasSelect || !b.HasPunct || b.Python != bundledPython(l.neural) || b.PythonSource != SourceBundled ||
		b.SelectSource != SourceBundled || b.PunctSource != SourceBundled ||
		b.SelectPackage != filepath.Join(l.neural, "select") || b.SelectWorker != filepath.Join(l.neural, "sequence_runtime.py") ||
		b.PunctPackage != filepath.Join(l.neural, "punct") || b.PunctWorker != filepath.Join(l.neural, "punct_runtime.py") {
		t.Fatalf("bundled layout: %+v", b)
	}
}

// 少了模型或 worker 的那一項不啟動，由統計引擎選字（或原文提交）。
func TestFindMissingModel(t *testing.T) {
	l := newFindLayout(t)
	touch(t, bundledPython(l.neural))
	touch(t, filepath.Join(l.neural, "sequence_runtime.py"))
	touch(t, filepath.Join(l.neural, "punct", "manifest.json"))
	b := Find(l.data)
	if b.HasSelect || b.SelectSource != "" || b.HasPunct || b.PunctSource != "" {
		t.Fatalf("missing model or worker must not start: %+v", b)
	}
}

// 缺少 numpy／onnxruntime 是持續的狀態：回報 runtime_missing，十分鐘才重試一次，不每幾秒啟動 Python。
func TestSupervisorRuntimeMissingStateAndSlowRetry(t *testing.T) {
	load := func(Config) (Manifest, error) { return Manifest{}, nil }
	var attempts atomic.Int32
	s, err := openSupervisedAsync(Config{}, load, func(c Config) (*Worker, error) {
		attempts.Add(1)
		return nil, &StartupError{Reason: ReasonRuntimeMissing, what: "neural worker"}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for deadline := time.Now().Add(5 * time.Second); s.State() != "runtime_missing"; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("state %q", s.State())
		}
	}
	s.mu.Lock()
	s.Cooldown = 0
	s.mu.Unlock()
	for i := 0; i < 3; i++ {
		if _, err := s.Score(request()); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("score: %v", err)
		}
	}
	time.Sleep(50 * time.Millisecond)
	s.mu.Lock()
	n := attempts.Load()
	s.lastFailure = time.Now().Add(-RuntimeMissingRetry) // 十分鐘後
	s.mu.Unlock()
	if n != 1 {
		t.Fatalf("retried %d times within cooldown", n-1)
	}
	s.Score(request())
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		s.mu.Lock()
		n, starting := attempts.Load(), s.starting
		s.mu.Unlock()
		if n == 2 && !starting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no retry after cooldown: attempts=%d", n)
		}
	}
}

func TestPunctRuntimeMissingState(t *testing.T) {
	dir, _ := punctPackage(t)
	missing := filepath.Join(t.TempDir(), "python3")
	w, err := openPunct(PunctConfig{Package: dir, Threads: 1, Timeout: 100 * time.Millisecond}, func() *exec.Cmd {
		return exec.Command(missing)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for deadline := time.Now().Add(5 * time.Second); w.State() != "runtime_missing"; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("state %q reason %q", w.State(), w.Reason())
		}
	}
	w.mu.Lock()
	w.stoppedAt = time.Now().Add(-10 * time.Second) // 一般的 3 秒冷卻已過
	w.mu.Unlock()
	if _, err := w.Punctuate("我想吃"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("punctuate: %v", err)
	}
	if w.State() != "runtime_missing" {
		t.Fatalf("retried before RuntimeMissingRetry: %s", w.State())
	}
}
