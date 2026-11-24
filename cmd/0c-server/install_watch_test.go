package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/0c-biggo/0c-ime/bridge/ipc"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
	"github.com/0c-biggo/0c-ime/internal/neural"
	"golang.org/x/sys/windows"
)

// 檔案換掉後要連續兩次看到同一個新狀態才交接（套件可能還沒裝完）；核對失敗的那一組不再重試，檔案再變才重新考慮。
func TestInstallWatchWaitsForStableFilesAndSkipsRejected(t *testing.T) {
	dir := t.TempDir()
	exe, manifest := filepath.Join(dir, "0c-server"), filepath.Join(dir, "manifest.json")
	missing := filepath.Join(dir, "neural", "select", "manifest.json")
	os.WriteFile(exe, []byte("v1"), 0o755)
	os.WriteFile(manifest, []byte("{}"), 0o644)
	start := time.Now()
	w := newInstallWatch([]string{exe, manifest, missing}, time.Minute, start)
	at := func(m int) time.Time { return start.Add(time.Duration(m) * time.Minute) }
	poll := func(m int) (bool, bool) { return w.poll(at(m)) }
	if r, c := w.poll(start.Add(30 * time.Second)); r || c {
		t.Fatal("polled before interval")
	}
	if r, c := poll(1); r || c {
		t.Fatal("unchanged files reported")
	}
	replace := func(path, body string) { // 像 dpkg：寫新檔再改名蓋過去（新 inode）
		os.WriteFile(path+".dpkg-new", []byte(body), 0o755)
		os.Rename(path+".dpkg-new", path)
	}
	replace(exe, "v2")
	if r, c := poll(2); r || !c {
		t.Fatalf("first sight of change: ready=%v changed=%v", r, c)
	}
	replace(manifest, `{"x":1}`) // 還在裝：又變了
	if r, c := poll(3); r || !c {
		t.Fatalf("still changing: ready=%v changed=%v", r, c)
	}
	if r, _ := poll(4); !r {
		t.Fatal("stable change not ready")
	}
	step, stamp := w.nextStep()
	if step != watchCheck {
		t.Fatalf("step %d", step)
	}
	if !w.finish(stamp, &checkError{err: errors.New("exit status 1")}, at(4)) {
		t.Fatal("definite failure not rejected")
	}
	for m := 5; m < 8; m++ {
		if r, _ := poll(m); r {
			t.Fatal("rejected files retried without another change")
		}
	}
	// 發行版另外裝了模型套件：原本不存在的檔案出現也算。
	os.MkdirAll(filepath.Dir(missing), 0o755)
	os.WriteFile(missing, []byte("{}"), 0o644)
	if r, c := poll(8); r || !c {
		t.Fatal("new file not noticed")
	}
	if r, _ := poll(9); !r {
		t.Fatal("new stable files not ready")
	}
}

// 寫一份已編譯的小字典（含 manifest.json）；回傳 manifest 的 SHA-256。
func writeCompiledData(t *testing.T, data string) string {
	t.Helper()
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	lex, err := lexicon.Build([]lexicon.Entry{{Surface: "麻婆", Reading: zhuyin.ParseReading("ㄇㄚˊ ㄆㄛˊ"), PRW: 1}})
	if err != nil {
		t.Fatal(err)
	}
	sys, err := lm.BuildCompact(map[string]float64{"麻婆": 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]any{}
	for name, body := range map[string][]byte{"lexicon.bin": lex.Encode(), "lm.bin": sys.Encode()} {
		if err := os.WriteFile(filepath.Join(data, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
		files[name] = map[string]string{"sha256": fmt.Sprintf("%x", sha256.Sum256(body))}
	}
	manifest, _ := json.Marshal(map[string]any{"format": 1, "files": files})
	if err := os.WriteFile(filepath.Join(data, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(manifest))
}

func healthOf(t *testing.T, sock string) map[string]string {
	t.Helper()
	c, err := dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if err := ipc.WriteFrame(c, ipc.Envelope{Op: ipc.OpHealth}); err != nil {
		t.Fatal(err)
	}
	var r ipc.Reply
	if err := ipc.ReadFrame(c, &r); err != nil || !r.OK {
		t.Fatalf("health: %+v %v", r, err)
	}
	return r.Info
}

// 系統安裝：沒有內附 Python 時用系統的 Python，模型可以放在個人目錄；找到模型卻沒有 Python
// 時回報 runtime_missing，照常用統計引擎打字。
func TestSystemInstallNeuralDiscovery(t *testing.T) {
	binary := buildServer(t)
	root := t.TempDir()
	data := filepath.Join(root, "share", "0c", "data")
	sha := writeCompiledData(t, data)
	neuralDir := filepath.Join(root, "share", "0c", "neural")
	os.MkdirAll(neuralDir, 0o755)
	os.WriteFile(filepath.Join(neuralDir, "sequence_runtime.py"), nil, 0o644)
	os.WriteFile(filepath.Join(neuralDir, "punct_runtime.py"), nil, 0o644)
	userDir := filepath.Join(root, "home", "zuin")
	// 核對得過的模型套件（雜湊與字典配對都對；模型內容是假的，worker 不會真的就緒）。
	for name, manifest := range map[string]map[string]any{
		"select": {"schema": "zuin-sequence-runtime-v1", "context_policy": "confirmed_left_64", "dictionary_sha256": sha, "weight": 1},
		"punct":  {"schema": neural.PunctSchema},
	} {
		p := filepath.Join(userDir, "models", sha, name)
		os.MkdirAll(p, 0o755)
		files := map[string]string{}
		for _, f := range []string{"model.onnx", "vocab.json"} {
			os.WriteFile(filepath.Join(p, f), []byte(f), 0o644)
			files[f] = fmt.Sprintf("%x", sha256.Sum256([]byte(f)))
		}
		manifest["files"] = files
		b, _ := json.Marshal(manifest)
		os.WriteFile(filepath.Join(p, "manifest.json"), b, 0o644)
	}
	os.WriteFile(filepath.Join(userDir, "settings.json"), []byte(`{"punctuation":true}`), 0o600)
	run := func(t *testing.T, python string) map[string]string {
		sock := testPipe("system-python")
		server, _ := startServer(t, binary, "-data", data, "-user", filepath.Join(userDir, "user.json"), "-socket", sock,
			"-system-python", python, "-install-watch", "off")
		defer stopServer(t, server)
		waitDial(t, sock)
		return healthOf(t, sock)
	}
	t.Run("沒有 Python", func(t *testing.T) {
		// 照樣以系統 Python 的路徑啟動 worker：執行檔不存在是 runtime_missing（10 分鐘重試，之後裝好 Python
		// 不必重啟引擎）；模型來源照實回報。
		sock := testPipe("no-python")
		server, _ := startServer(t, binary, "-data", data, "-user", filepath.Join(userDir, "user.json"), "-socket", sock,
			"-system-python", filepath.Join(root, "no", "python.exe"), "-install-watch", "off")
		defer stopServer(t, server)
		waitDial(t, sock)
		info := healthOf(t, sock)
		for deadline := time.Now().Add(5 * time.Second); info["selection_state"] == "loading" || info["punctuation_state"] == "loading"; info = healthOf(t, sock) {
			if time.Now().After(deadline) {
				t.Fatalf("still loading: %v", info)
			}
			time.Sleep(20 * time.Millisecond)
		}
		want := map[string]string{"neural_startup_state": "runtime_missing", "selection_state": "runtime_missing",
			"punctuation_state": "runtime_missing", "neural_python": "missing", "selection_source": "user", "punctuation_source": "user"}
		for k, v := range want {
			if info[k] != v {
				t.Errorf("%s = %q，應為 %q（%v）", k, info[k], v, info)
			}
		}
	})
	t.Run("系統 Python 與個人目錄的模型", func(t *testing.T) {
		// 存在的執行檔；不認得 Python 的參數，啟動後沒有就緒就退回
		info := run(t, filepath.Join(os.Getenv("SystemRoot"), "System32", "whoami.exe"))
		want := map[string]string{"neural_python": "system", "neural_startup_state": "enabled", "selection_source": "user", "punctuation_source": "user"}
		for k, v := range want {
			if info[k] != v {
				t.Errorf("%s = %q，應為 %q（%v）", k, info[k], v, info)
			}
		}
	})
}

// 測試用的 stderr 收集器：引擎還在跑時就要讀，讀寫要上鎖。
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuffer) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

// installFile 換掉安裝的檔案：執行中的程式不能被蓋過去，先把舊檔改名移開（執行中的引擎留著舊檔），再放新檔。
func installFile(t *testing.T, src, dst string) {
	t.Helper()
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, fmt.Sprintf("%s.old-%d", dst, time.Now().UnixNano())); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(dst, body, 0o755); err != nil {
		t.Fatal(err)
	}
}

// 核對通過的那一組記下來：交接逾時（一直在組字）之後直接再交接，不再跑一次 -check-data；核對進行中不重複開始。
// 暫時性失敗（逾時、執行不了）以加倍的間隔重試，次數用完才拒絕；確定載入不了立刻拒絕到檔案再變。
func TestInstallWatchRemembersPassedCheckAndRetriesTransientFailures(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "0c-server")
	os.WriteFile(exe, []byte("v1"), 0o755)
	start := time.Now()
	at := func(d time.Duration) time.Time { return start.Add(d) }
	w := newInstallWatch([]string{exe}, time.Minute, start)
	os.WriteFile(exe+".new", []byte("v2 longer"), 0o755)
	os.Rename(exe+".new", exe)
	w.poll(at(time.Minute))
	if r, _ := w.poll(at(2 * time.Minute)); !r {
		t.Fatal("stable change not ready")
	}
	step, stamp := w.nextStep()
	if step != watchCheck {
		t.Fatalf("first step %d", step)
	}
	if again, _ := w.nextStep(); again != watchWait {
		t.Fatalf("second check started while the first is running: %d", again)
	}
	transient := &checkError{err: context.DeadlineExceeded, transient: true}
	now := at(2 * time.Minute)
	for i := 1; i < installCheckRetries; i++ {
		if w.finish(stamp, transient, now) {
			t.Fatalf("transient failure %d rejected", i)
		}
		if r, _ := w.poll(now.Add(w.interval<<i - time.Second)); r {
			t.Fatalf("retry %d before backoff", i)
		}
		now = now.Add(w.interval << i)
		if r, _ := w.poll(now); !r {
			t.Fatalf("retry %d not ready after backoff", i)
		}
		if step, stamp = w.nextStep(); step != watchCheck {
			t.Fatalf("retry %d step %d", i, step)
		}
	}
	if !w.finish(stamp, transient, now) {
		t.Fatal("transient failures not rejected after the retries ran out")
	}
	if r, _ := w.poll(now.Add(time.Hour)); r {
		t.Fatal("rejected files retried")
	}
	// 檔案又變了（套件修好重裝）：重新核對，通過後下一次 poll 立刻交接；交接逾時後再交接也不必再核對。
	os.WriteFile(exe+".new", []byte("v3 even longer"), 0o755)
	os.Rename(exe+".new", exe)
	now = now.Add(2 * time.Hour)
	w.poll(now)
	now = now.Add(time.Minute)
	if r, _ := w.poll(now); !r {
		t.Fatal("new files not ready")
	}
	if step, stamp = w.nextStep(); step != watchCheck {
		t.Fatalf("step %d", step)
	}
	if w.finish(stamp, nil, now.Add(10*time.Second)) {
		t.Fatal("passed check rejected")
	}
	if r, _ := w.poll(now.Add(10 * time.Second)); !r {
		t.Fatal("passed check did not hand over on the next tick")
	}
	for i := 0; i < 3; i++ { // 交接逾時之後的每一次
		if step, _ := w.nextStep(); step != watchHandOver {
			t.Fatalf("passed files checked again: step %d", step)
		}
	}
}

const fakeEngineEnv = "ZC_TEST_FAKE_ENGINE"

// fakeEngineMain 讓測試執行檔扮演被換上來的新執行檔（TestMain 依 fakeEngineEnv 轉來這裡）。
func fakeEngineMain(mode string) int {
	switch mode {
	case "fails": // 自己回報載入失敗
		fmt.Fprintln(os.Stderr, strings.Repeat("字", 150)) // 450 位元組
		return 1
	case "slow": // 卡住，直到叫它的行程結束：不留下占著暫存目錄裡執行檔的行程
		if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(os.Getppid())); err == nil {
			windows.WaitForSingleObject(h, uint32((5 * time.Minute).Milliseconds()))
		}
		return 0
	default:
		fmt.Println("資料包可載入")
		return 0
	}
}

// -check-data 的失敗分兩種：新程式自己回報載入失敗是確定的；逾時、執行不了是暫時的。輸出依 UTF-8 字元截斷。
func TestCheckNewEngineClassifiesFailures(t *testing.T) {
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("字", 150)
	var check *checkError
	t.Setenv(fakeEngineEnv, "fails")
	err = checkNewEngine(context.Background(), self, dir, time.Minute)
	if !errors.As(err, &check) || check.transient {
		t.Fatalf("load failure: %#v", err)
	}
	if !utf8.ValidString(check.out) || len(check.out) > 300 || !strings.HasPrefix(long, check.out) || len(check.out) != 300 {
		t.Fatalf("output cut in the middle of a character: %d bytes %q", len(check.out), check.out)
	}
	t.Setenv(fakeEngineEnv, "slow")
	started := time.Now()
	if err := checkNewEngine(context.Background(), self, dir, 200*time.Millisecond); !errors.As(err, &check) || !check.transient {
		t.Fatalf("timeout: %#v", err)
	}
	if time.Since(started) > 10*time.Second {
		t.Fatal("timeout not enforced")
	}
	if err := checkNewEngine(context.Background(), filepath.Join(dir, "missing.exe"), dir, time.Minute); !errors.As(err, &check) || !check.transient {
		t.Fatalf("not executable: %#v", err)
	}
	t.Setenv(fakeEngineEnv, "ok")
	if err := checkNewEngine(context.Background(), self, dir, time.Minute); err != nil {
		t.Fatal(err)
	}
	if got := truncateUTF8("ab字", 3); got != "ab" {
		t.Fatalf("truncateUTF8 = %q", got)
	}
}

// 交接時收到結束訊號：不論訊號已經處理完、還是剛送進通道還沒處理，stop 都要回報收到（不能 exec）。
func TestWatchSignalsNeverMissesASignalBeforeStop(t *testing.T) {
	for i := 0; i < 2000; i++ {
		sig := make(chan os.Signal, 1)
		var calls atomic.Int32
		stop := watchSignals(sig, func() { calls.Add(1) }, func(chan<- os.Signal) {})
		if i%2 == 0 {
			sig <- syscall.SIGTERM // 剛到、處理的 goroutine 可能還沒拿到
		}
		if got := stop(); got != (i%2 == 0) {
			t.Fatalf("round %d: signaled = %v", i, got)
		}
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	if watchSignals(sig, func() {}, signal.Stop)() {
		t.Fatal("no signal but reported one")
	}
}

// 新執行檔的核對在背景跑：核對卡住（最長 2 分鐘）時，引擎照樣依 -idle-exit 結束，不被擋住。
func TestSystemInstallCheckDoesNotBlockTheEngineLoop(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "libexec", "0c", "0c-server.exe")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	installFile(t, buildServer(t), exe)
	data := filepath.Join(root, "share", "0c", "data")
	writeCompiledData(t, data)
	user := filepath.Join(root, "home", "user.json")
	os.MkdirAll(filepath.Dir(user), 0o700)
	sock := testPipe("check-blocks")
	logs := &lockedBuffer{}
	server := serverCommand(exe, "-data", data, "-user", user, "-socket", sock, "-idle-exit", "3s",
		"-install-watch", "on", "-install-watch-interval", "200ms", "-install-watch-idle", "0s")
	server.Env = append(os.Environ(), fakeEngineEnv+"=slow") // 換上來的「新執行檔」是卡住的測試執行檔
	server.Stderr = logs
	launch(t, server)
	waitDial(t, sock)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	installFile(t, self, exe)
	exited, _ := waitFor(server, 20*time.Second)
	if !exited {
		t.Fatalf("engine loop blocked by the check:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "閒置") {
		t.Fatalf("engine did not exit by idle:\n%s", logs.String())
	}
}
